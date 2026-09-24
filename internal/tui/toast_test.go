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
	if got := len(tm.lines()); got != 3 {
		t.Fatalf("líneas = %d, want 3 apiladas", got)
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

// El splice preserva el texto a la derecha del toast y no corrompe el ANSI.
func TestOverlayANSIYSafe(t *testing.T) {
	base := strings.Join([]string{
		pad("line0", 40),
		"\x1b[31m" + pad("aaaa", 40) + "\x1b[0m",
		pad("line2", 38) + "ZZ",
		pad("line3", 40),
	}, "\n")
	out := overlayToasts(base, []string{"\x1b[32mOK\x1b[0m"}, 40, 4, 1)
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
	plano := ansi.Strip(lines[2])
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
}

// El overlay reserva las filas inferiores (keybinds) y no las toca.
func TestOverlayReservaKeybinds(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2", "k0", "k1"}, "\n")
	out := overlayToasts(base, []string{"TOAST"}, 20, 5, 2)
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
	out := overlayToasts(base, []string{"viejo", "nuevo"}, 20, 3, 2)
	lines := strings.Split(out, "\n")
	plano := ansi.Strip(lines[0])
	if !strings.Contains(plano, "nuevo") {
		t.Errorf("el toast más reciente no se muestra: %q", plano)
	}
	if strings.Contains(stripANSI(out), "viejo") {
		t.Errorf("no debería caber el toast antiguo:\n%s", out)
	}
}

// Overlay sin ancho o sin toasts es no-op.
func TestOverlayNoop(t *testing.T) {
	if got := overlayToasts("x", nil, 10, 5, 0); got != "x" {
		t.Errorf("sin toasts = %q, want x", got)
	}
	if got := overlayToasts("x", []string{"t"}, 0, 5, 0); got != "x" {
		t.Errorf("sin ancho = %q, want x", got)
	}
}
