package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestToastShowYExpira(t *testing.T) {
	var tm toastManager
	tm.showSuccess("pull ok")
	if len(tm.toasts) != 1 {
		t.Fatalf("toasts = %d, want 1", len(tm.toasts))
	}
	// tras superar la duración, update lo descarta
	tm.toasts[0].created = time.Now().Add(-toastDuration - time.Second)
	tm.update()
	if len(tm.toasts) != 0 {
		t.Errorf("el toast no expiró: %d vivos", len(tm.toasts))
	}
}

func TestToastApilado(t *testing.T) {
	var tm toastManager
	tm.showInfo("a")
	tm.showWarning("b")
	tm.showError("c")
	if got := len(tm.blocks()); got != 3 {
		t.Fatalf("bloques = %d, want 3 apiladas", got)
	}
	if got := len(tm.lines()); got != 3 {
		t.Fatalf("líneas = %d, want 3 (una por toast corto)", got)
	}
}

// Un mensaje largo (p.ej. acción fallida con hint) se envuelve en varias líneas
// y el texto completo —incluido el hint— queda visible.
func TestToastWrapMuestraHintCompleto(t *testing.T) {
	msg := "pull failed diverged-node: fatal: Not possible to fast-forward — diverged? pull --rebase manual"
	var tm toastManager
	tm.showError(msg)

	block := tm.blocks()[0]
	if len(block) < 2 {
		t.Fatalf("esperaba word-wrap en varias líneas, got %d", len(block))
	}
	plano := collapse(strings.Join(block, "\n"))
	if !strings.Contains(plano, "pull --rebase manual") {
		t.Errorf("el hint accionable no es visible:\n%s", plano)
	}
	if !strings.Contains(plano, "Not possible to fast-forward") {
		t.Errorf("el motivo se perdió:\n%s", plano)
	}
	for i, l := range block {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("línea %d ancho = %d, want <= %d", i, w, toastMaxWidth)
		}
	}
}

// Una palabra sin espacios más ancha que el toast se corta duro, sin desbordar.
func TestToastPalabraLargaNoDesborda(t *testing.T) {
	var tm toastManager
	tm.showError(strings.Repeat("x", 500))
	for i, l := range tm.lines() {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("línea %d ancho = %d, want <= %d", i, w, toastMaxWidth)
		}
	}
	if got := ansi.Strip(strings.Join(tm.lines(), "")); strings.Contains(got, "…") {
		t.Errorf("el wrap no debe truncar con elipsis: %q", got)
	}
}

func TestToastAnchoClampado(t *testing.T) {
	var tm toastManager
	tm.showError(strings.Repeat("x", 500))
	line := tm.lines()[0]
	if w := ansi.StringWidth(line); w > toastMaxWidth {
		t.Errorf("ancho = %d, want <= %d", w, toastMaxWidth)
	}
	var tm2 toastManager
	tm2.showInfo("ok")
	if w := ansi.StringWidth(tm2.lines()[0]); w < toastMinWidth {
		t.Errorf("ancho = %d, want >= %d", w, toastMinWidth)
	}
}

func TestToastIconos(t *testing.T) {
	cases := map[toastLevel]string{
		toastSuccess: "✓", toastError: "✗", toastInfo: "ℹ", toastWarning: "⚠",
	}
	for level, icon := range cases {
		if got := toastIcon(level); got != icon {
			t.Errorf("icono(%d) = %q, want %q", level, got, icon)
		}
	}
}

// El splice preserva el texto a la derecha del toast y no corrompe el ANSI de
// la línea de debajo (que llega con SGR abierto).
func TestOverlayANSIYSafe(t *testing.T) {
	base := strings.Join([]string{
		pad("line0", 40),
		pad("line1", 40),
		"\x1b[31m" + pad("line2", 38) + "ZZ" + "\x1b[0m",
		pad("line3", 40),
	}, "\n")
	out := overlayToasts(base, [][]string{{"\x1b[32mOK\x1b[0m"}}, 40, 4, 1)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("líneas = %d, want 4", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 40 {
			t.Errorf("línea %d ancho = %d, want 40", i, w)
		}
	}
	// y = 4-1-1 = 2
	spliced := lines[2]
	plano := ansi.Strip(spliced)
	if !strings.Contains(plano, "OK") {
		t.Errorf("toast ausente en la esquina inferior derecha: %q", plano)
	}
	if !strings.Contains(plano, "line2") {
		t.Errorf("se perdió el texto de la izquierda: %q", plano)
	}
	// la cola derecha de la línea base (la "Z") sobrevive al splice
	if !strings.HasSuffix(plano, "Z") {
		t.Errorf("no se preservó el texto a la derecha del toast: %q", plano)
	}
	// el SGR de la base sigue presente (izquierda roja) y el del toast (verde)
	if !strings.Contains(spliced, "\x1b[31m") {
		t.Errorf("se perdió el color de la línea de debajo: %q", spliced)
	}
	if !strings.Contains(spliced, "\x1b[32m") {
		t.Errorf("se perdió el color del toast: %q", spliced)
	}
}

// El overlay acota el ancho del toast al de la terminal: ninguna línea supera
// el ancho disponible.
func TestOverlayClampaAnchoTerminal(t *testing.T) {
	base := strings.Join(sliceOf(pad("contenido", 20), 20), "\n")
	var tm toastManager
	tm.showError(strings.Repeat("palabra ", 30)) // mensaje largo → toast de 60
	blocks := tm.blocks()
	if blockWidth(blocks[0]) <= 20 {
		t.Fatalf("precondición: el toast debería superar el ancho de terminal")
	}
	out := overlayToasts(base, blocks, 20, 20, 0)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 20 {
			t.Errorf("línea %d ancho = %d > 20: %q", i, w, ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(out), "palabra") {
		t.Errorf("el toast no se dibujó:\n%s", ansi.Strip(out))
	}
}

// El overlay reserva las filas inferiores (keybinds) y no las toca.
func TestOverlayReservaKeybinds(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2", "k0", "k1"}, "\n")
	out := overlayToasts(base, [][]string{{"TOAST"}}, 20, 5, 2)
	lines := strings.Split(out, "\n")
	if lines[3] != "k0" || lines[4] != "k1" {
		t.Errorf("se tapó la sección de keybinds: %q", lines[3:])
	}
	if !strings.Contains(ansi.Strip(lines[2]), "TOAST") {
		t.Errorf("el toast no se apiló sobre las filas reservadas: %q", ansi.Strip(lines[2]))
	}
}

// Sin sitio, se prioriza el toast más reciente y nunca se sale por arriba.
func TestOverlaySinEspacioPriorizaReciente(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	out := overlayToasts(base, [][]string{{"viejo"}, {"nuevo"}}, 20, 3, 2)
	lines := strings.Split(out, "\n")
	plano := ansi.Strip(lines[0])
	if !strings.Contains(plano, "nuevo") {
		t.Errorf("el toast más reciente no se muestra: %q", plano)
	}
	if strings.Contains(stripANSI(out), "viejo") {
		t.Errorf("no debería caber el toast antiguo:\n%s", out)
	}
}

// Un toast de varias líneas se apila sin desbordar por arriba ni tapar keybinds.
func TestOverlayToastMultilinea(t *testing.T) {
	var tm toastManager
	tm.showError("pull failed node: fatal: Not possible to fast-forward — diverged? pull --rebase manual")
	out := overlayToasts(strings.Join(sliceOf(pad("x", 60), 8), "\n"), tm.blocks(), 60, 8, 5)
	lines := strings.Split(out, "\n")
	if len(lines) != 8 {
		t.Fatalf("líneas = %d, want 8", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 60 {
			t.Errorf("línea %d ancho = %d > 60", i, w)
		}
	}
	if !strings.Contains(collapse(strings.Join(lines, "\n")), "pull --rebase manual") {
		t.Errorf("el toast multilínea no se dibujó completo:\n%s", strings.Join(lines, "\n"))
	}
}

// En terminales estrechas el toast se re-envuelve (no se trunca): el hint
// accionable sigue visible.
func TestToastReEnvuelveEnAnchoEstrecho(t *testing.T) {
	var tm toastManager
	tm.showError("pull failed diverged-node: fatal: Not possible to fast-forward — diverged? pull --rebase manual")
	blocks := tm.blocksFor(40)
	for i, l := range blocks[0] {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("línea %d ancho = %d > 40", i, w)
		}
	}
	plano := collapse(strings.Join(blocks[0], "\n"))
	if !strings.Contains(plano, "pull --rebase manual") {
		t.Errorf("el hint accionable se perdió al re-envolver:\n%s", plano)
	}
}

// Un toast más alto que el hueco disponible dibuja sus últimas líneas en vez de
// desaparecer.
func TestOverlayBloqueMasAltoQueHueco(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	block := []string{"t0", "t1", "t2", "t3", "t4"}
	out := overlayToasts(base, [][]string{block}, 20, 3, 0)
	lines := strings.Split(out, "\n")
	plano := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(plano, "t4") {
		t.Errorf("no se dibujó el cierre del toast:\n%s", plano)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 20 {
			t.Errorf("línea %d ancho = %d > 20", i, w)
		}
	}
}

// wrapText siempre avanza, incluso con maxWidth menor que una runa ancha
// (evita bucle infinito).
func TestWrapTextAnchoMenorQueRuna(t *testing.T) {
	lines := wrapText("日本語", 1)
	if got := collapse(strings.Join(lines, "")); got != "日本語" {
		t.Errorf("se perdió texto: %q", got)
	}
}

// Un mensaje con saltos de línea (p.ej. err.Error() multilínea) se dibuja como
// varias líneas del bloque sin romper el splice por líneas.
func TestToastConSaltosDeLineaNoRompeSplice(t *testing.T) {
	var tm toastManager
	tm.showError("fatal: primera\r\nsegunda línea")
	blocks := tm.blocksFor(60)
	if len(blocks[0]) != 2 {
		t.Fatalf("líneas = %d, want 2 (una por salto)", len(blocks[0]))
	}
	out := overlayToasts(strings.Join(sliceOf(pad("x", 60), 6), "\n"), blocks, 60, 6, 2)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("línea %d ancho = %d, want 60", i, w)
		}
	}
	plano := collapse(out)
	if !strings.Contains(plano, "primera") || !strings.Contains(plano, "segunda") {
		t.Errorf("se perdió texto del mensaje multilínea:\n%s", plano)
	}
}

// sliceOf repite s n veces.
func sliceOf(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// collapse quita ANSI y normaliza espacios/saltos para comparar frases que el
// word-wrap pudo partir entre líneas.
func collapse(s string) string {
	return strings.Join(strings.Fields(ansi.Strip(s)), " ")
}

// Overlay sin ancho o sin toasts es no-op.
func TestOverlayNoop(t *testing.T) {
	if got := overlayToasts("x", nil, 10, 5, 0); got != "x" {
		t.Errorf("sin toasts = %q, want x", got)
	}
	if got := overlayToasts("x", [][]string{{"t"}}, 0, 5, 0); got != "x" {
		t.Errorf("sin ancho = %q, want x", got)
	}
}
