// Package cmdlog registra los procesos que gitdash lanza (subprocess de git y
// handoffs de terminal) para poder auditar qué se ejecutó de verdad y con qué
// resultado.
//
// Existe por un motivo concreto: con la política de pull delegada en el
// gitconfig del usuario, el argv no basta. `git pull` a secas puede haber
// integrado con merge, con rebase o con rebase+autostash según lo que digan
// `pull.rebase` y `branch.<name>.rebase`, y el argv es idéntico en los tres
// casos. El log guarda el argv Y el resultado, que es lo que git delata en su
// propia salida.
//
// El log vive solo en memoria y por sesión (ring buffer acotado): cero efectos
// secundarios, nada que limpiar. El recorder es un global con default no-op
// porque los puntos de exec están en dos paquetes distintos (gitstatus y tui) y
// enhebrar un recorder por todas las firmas de StreamPool/Collect/Run no
// aportaría nada.
package cmdlog

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultCap es el número de entradas que guarda el ring por defecto.
const DefaultCap = 500

// Class clasifica una entrada por su ruido, para que el panel pueda filtrar: las
// lecturas del scan son ~4 por repo y cada una repite la misma interrogación
// (`status`, `log`, `worktree list`, `rev-list`), así que se ocultan por
// defecto.
type Class int

const (
	// ClassRead son las lecturas del scan (status, log, worktree list,
	// rev-list, rev-parse): informativas pero ruidosas.
	ClassRead Class = iota
	// ClassAction es lo que una tecla del usuario lanzó sobre un repo.
	ClassAction
	// ClassAuto es el fetch que dispara el scan sin que nadie lo pida.
	ClassAuto
)

// String etiqueta la clase para el panel.
func (c Class) String() string {
	switch c {
	case ClassAction:
		return "action"
	case ClassAuto:
		return "auto"
	default:
		return "read"
	}
}

// Entry es una línea del log: o una intención (una tecla) o una ejecución (un
// proceso terminado). Se guardan ambas porque la intención es lo único que
// distingue "pulsé p y elegí rebase" de "el gitconfig decidió por mí": el argv
// de las dos es el mismo.
type Entry struct {
	// Seq es monótono y conserva el orden real de ejecución cuando varias
	// acciones vuelan a la vez (batches de fetch).
	Seq int
	At  time.Time

	// Intent marca la línea como "qué pediste" en vez de "qué corrió".
	Intent bool

	Class Class
	// Repo es el nombre visible del repo (basename del directorio), no el
	// path: el panel se lee junto a la tabla, que ya muestra nombres.
	Repo string
	// Dir es el path absoluto del repo; solo en ejecuciones.
	Dir string
	// Key es la tecla que lo disparó; solo en intenciones.
	Key string
	// Action etiqueta la acción (pull, pull_rebase, push, fetch,
	// worktree_remove, cmd, editor, lazygit, shell).
	Action string
	// Argv es el argv tal cual se ejecutó, programa incluido. Vacío en
	// intenciones.
	Argv []string
	// Exit es el código de salida, o -1 si no hubo proceso que medir
	// (intenciones, handoffs de terminal sin código de salida).
	Exit int
	// Dur es la duración del proceso. 0 en intenciones y en handoffs de
	// terminal: ahí bubbletea presta la terminal al hijo y medirlo exigiría
	// guardar el instante de arranque en el modelo.
	Dur time.Duration
	// Outcome es la clasificación de la salida de git (rebase+autostash,
	// merge, fast-forward, up-to-date, diverged, rebase-conflict, failed).
	// Vacío cuando la salida no dice nada útil.
	Outcome string
}

// Command renderiza el argv como una línea de comando legible.
func (e Entry) Command() string { return strings.Join(e.Argv, " ") }

// Recorder es el ring acotado de entradas. Seguro para uso concurrente: los
// execs de git corren en paralelo (pool de 8 en el scan, 4 por batch de fetch)
// y el ring se escribe desde todas esas goroutines a la vez.
type Recorder struct {
	mu    sync.Mutex
	buf   []Entry
	next  int // dónde escribir la siguiente entrada
	count int // cuántas hay vivas (≤ len(buf))
	seq   int
}

// New crea un Recorder con capacidad cap (DefaultCap si cap <= 0).
func New(cap int) *Recorder {
	if cap <= 0 {
		cap = DefaultCap
	}
	return &Recorder{buf: make([]Entry, cap)}
}

// add escribe una entrada pisando la más antigua cuando el ring está lleno.
func (r *Recorder) add(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
}

// Entries devuelve una copia de las entradas vivas en orden cronológico.
func (r *Recorder) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, r.count)
	start := (r.next - r.count + len(r.buf)) % len(r.buf)
	for i := range r.count {
		out = append(out, r.buf[(start+i)%len(r.buf)])
	}
	return out
}

// LastSeq devuelve el número de secuencia de la última entrada registrada, o 0
// si no hay ninguna. El panel lo usa para saber si su copia cacheada está
// obsoleta sin re-copiar el ring en cada frame.
func (r *Recorder) LastSeq() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}

// current es el recorder global. Puntero atómico en vez de mutex porque la
// escritura está en el camino caliente de cada exec, y nil significa
// "logging desactivado" (el caso por defecto fuera de la TUI).
var current atomic.Pointer[Recorder]

// SetRecorder instala el recorder global; nil lo desactiva. Lo llama la TUI al
// construirse y los tests lo restauran con t.Cleanup.
func SetRecorder(r *Recorder) { current.Store(r) }

// Active devuelve el recorder global, o nil si el logging está desactivado.
func Active() *Recorder { return current.Load() }

// addExec normaliza y añade una ejecución.
func (r *Recorder) addExec(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	r.add(e)
}

// addIntent normaliza y añade una intención.
func (r *Recorder) addIntent(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	e.Intent = true
	e.Exit = -1
	r.add(e)
}

// RecordExec registra una ejecución en el recorder global (no-op si no hay).
// Quien llama debe rellenar Exit: el código de salida del proceso.
func RecordExec(e Entry) {
	if r := current.Load(); r != nil {
		r.addExec(e)
	}
}

// RecordIntent registra una intención en el recorder global (no-op si no hay).
func RecordIntent(e Entry) {
	if r := current.Load(); r != nil {
		r.addIntent(e)
	}
}

// Entries devuelve las entradas del recorder global, o nil si no hay ninguno.
func Entries() []Entry {
	if r := current.Load(); r != nil {
		return r.Entries()
	}
	return nil
}

// LastSeq devuelve la última secuencia del recorder global, o 0.
func LastSeq() int {
	if r := current.Load(); r != nil {
		return r.LastSeq()
	}
	return 0
}
