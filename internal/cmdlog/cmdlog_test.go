package cmdlog

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// El ring se escribe desde muchas goroutines a la vez (los execs de git corren
// en paralelo: pool de 8 en el scan, 4 por batch de fetch). Con -race en CI,
// este test es el que garantiza que el ring no se corrompe.
func TestRecorderConcurrente(t *testing.T) {
	const n = 200
	rec := New(n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.addExec(Entry{Action: "pull", Argv: []string{"git", "pull"}, Exit: i % 2})
		}()
	}
	wg.Wait()

	entries := rec.Entries()
	if len(entries) != n {
		t.Fatalf("entradas = %d, want %d", len(entries), n)
	}
	// Secuencias monótonas y sin repetir: es lo que permite reconstruir el
	// orden real cuando las acciones se solapan.
	seen := make(map[int]bool, n)
	for i, e := range entries {
		if e.Seq != i+1 {
			t.Errorf("entradas[%d].Seq = %d, want %d", i, e.Seq, i+1)
		}
		if seen[e.Seq] {
			t.Errorf("Seq %d repetida", e.Seq)
		}
		seen[e.Seq] = true
	}
}

// Al llenarse el ring se pisa lo más antiguo, y lo que sobrevive sigue en orden
// cronológico (el panel lee la cola).
func TestRingPisaLoAntiguo(t *testing.T) {
	rec := New(3)
	for i := range 5 {
		rec.addExec(Entry{Action: fmt.Sprintf("a%d", i)})
	}
	entries := rec.Entries()
	if len(entries) != 3 {
		t.Fatalf("entradas = %d, want 3 (capacidad del ring)", len(entries))
	}
	for i, want := range []string{"a2", "a3", "a4"} {
		if entries[i].Action != want {
			t.Errorf("entradas[%d] = %q, want %q", i, entries[i].Action, want)
		}
	}
	if entries[0].Seq != 3 {
		t.Errorf("Seq de la primera viva = %d, want 3", entries[0].Seq)
	}
}

// New(0) cae al default en vez de crear un ring de capacidad 0 (que dejaría al
// panel sin nada que pintar y provocaría división por cero al indexar).
func TestNewCapacidadPorDefecto(t *testing.T) {
	rec := New(0)
	rec.addExec(Entry{Action: "pull"})
	if got := len(rec.Entries()); got != 1 {
		t.Fatalf("entradas = %d, want 1", got)
	}
}

// Sin recorder global, registrar es un no-op: es lo que permite que --print y
// los tests de otros packages no arrastren el log.
func TestSinRecorderGlobalNoRompe(t *testing.T) {
	SetRecorder(nil)
	if Active() != nil {
		t.Fatal("Active() debería ser nil tras SetRecorder(nil)")
	}
	RecordExec(Entry{Action: "pull", Argv: []string{"git", "pull"}})
	RecordIntent(Entry{Action: "pull", Key: "p"})
	if got := Entries(); got != nil {
		t.Errorf("Entries() = %v, want nil sin recorder", got)
	}
	if got := LastSeq(); got != 0 {
		t.Errorf("LastSeq() = %d, want 0 sin recorder", got)
	}
}

// SetRecorder(nil) se llama desde t.Cleanup en cuanto un test toca el global:
// si otro test del package se ejecuta después, no debe heredar el recorder.
func TestSetRecorderEsRestaurable(t *testing.T) {
	rec := New(4)
	SetRecorder(rec)
	t.Cleanup(func() { SetRecorder(nil) })
	RecordExec(Entry{Action: "push", Argv: []string{"git", "push"}})
	if got := LastSeq(); got != 1 {
		t.Fatalf("LastSeq() = %d, want 1", got)
	}
	if Active() != rec {
		t.Error("Active() no devuelve el recorder instalado")
	}
}

// Una intención se marca como tal y sin veredicto: no ha corrido nada, y
// Exit -1 la distingue de un proceso que salió con 0.
func TestIntentNoTieneVeredicto(t *testing.T) {
	rec := New(4)
	rec.addIntent(Entry{Action: "pull", Key: "p"})
	e := rec.Entries()[0]
	if !e.Intent {
		t.Error("Intent = false, want true")
	}
	if e.Exit != -1 {
		t.Errorf("Exit = %d, want -1 (nada ejecutado)", e.Exit)
	}
	if e.At.IsZero() {
		t.Error("At sin rellenar: el recorder debe fechar la entrada")
	}
}

// Una ejecución con Exit 0 explícito se queda en 0 (el 0 de un pull bien
// integrado es un dato, no un "sin informar").
func TestExecConservaExitCero(t *testing.T) {
	rec := New(4)
	rec.addExec(Entry{Action: "pull", Argv: []string{"git", "pull"}, Exit: 0, Outcome: "rebase"})
	e := rec.Entries()[0]
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Intent {
		t.Error("Intent = true en una ejecución")
	}
}

func TestCommandRenderizaArgv(t *testing.T) {
	e := Entry{Argv: []string{"git", "pull", "--rebase", "--autostash"}}
	if got, want := e.Command(), "git pull --rebase --autostash"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if got := (Entry{}).Command(); got != "" {
		t.Errorf("Command() de una entrada sin argv = %q, want %q", got, "")
	}
}

func TestClassString(t *testing.T) {
	for _, tc := range []struct {
		class Class
		want  string
	}{
		{ClassRead, "read"},
		{ClassAction, "action"},
		{ClassAuto, "auto"},
		{Class(99), "read"}, // valor desconocido cae en read, no en vacío
	} {
		if got := tc.class.String(); got != tc.want {
			t.Errorf("Class(%d).String() = %q, want %q", tc.class, got, tc.want)
		}
	}
}

// El At de una entrada lo pone el recorder, no quien llama: la marca de tiempo
// tiene que ser del momento de registrar, no de construir la struct.
func TestAddIgnoraAtCero(t *testing.T) {
	rec := New(2)
	antes := time.Now()
	rec.addExec(Entry{Action: "pull"})
	got := rec.Entries()[0].At
	if got.Before(antes) {
		t.Errorf("At = %v, anterior al momento del registro", got)
	}
}
