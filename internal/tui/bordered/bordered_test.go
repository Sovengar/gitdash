package bordered

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderWithTitleAnchoExactoYGrifos(t *testing.T) {
	out := RenderWithTitle(Rounded(), lipgloss.Color("238"), " gitdash ", "hola", 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3 (borde + contenido + borde)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("línea %d ancho = %d, want 20: %q", i, w, l)
		}
	}
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("esquinas superiores incorrectas: %q", top)
	}
	if !strings.Contains(top, " gitdash ") {
		t.Errorf("título no embebido en la línea superior: %q", top)
	}
	bot := ansi.Strip(lines[2])
	if !strings.HasPrefix(bot, "╰") || !strings.HasSuffix(bot, "╯") {
		t.Errorf("esquinas inferiores incorrectas: %q", bot)
	}
}

// Sin color de borde no se emite NINGÚN escape: el estilo del borde solo existe
// si hay un color. `ansi.Style` con un color nil no es "sin estilo", es un
// `\x1b[39m` (fg por defecto) alrededor de cada trozo, así que este caso
// distingue un borde sin pintar de uno pintado de blanco.
func TestRenderSinColorNoEmiteANSI(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, " titulo ", "contenido", 24)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("borde sin color emitió ANSI: %q", out)
	}
	if !strings.Contains(ansi.Strip(out), " titulo ") {
		t.Errorf("el título se perdió al no pintar: %q", out)
	}
}

// El relleno horizontal de la línea de borde es el del Border (fill), no un
// espacio: un borde sin fill propio tiene que caer a un espacio, y uno con fill
// lo conserva.
func TestRenderUsaElFillDelBorder(t *testing.T) {
	t.Run("fill propio", func(t *testing.T) {
		out := RenderWithTitle(Rounded(), nil, " t ", "c", 12)
		top := ansi.Strip(strings.Split(out, "\n")[0])
		if !strings.Contains(top, "─") {
			t.Errorf("relleno del borde = %q, want el fill ─ del Border", top)
		}
	})
	t.Run("fill vacío", func(t *testing.T) {
		b := lipgloss.Border{TopLeft: "|", Top: "", TopRight: "|", BottomLeft: "|", Bottom: "", BottomRight: "|", Left: "!", Right: "!"}
		out := RenderWithTitle(b, nil, "", "c", 10)
		lines := strings.Split(out, "\n")
		if got := ansi.Strip(lines[0]); got != "|        |" {
			t.Errorf("línea superior = %q, want | + espacios + |", got)
		}
		if got := ansi.Strip(lines[1]); got != "!c       !" {
			t.Errorf("línea de contenido = %q, want bordes laterales del Border", got)
		}
	})
}

// El contenido más ancho que el interior se recorta, no se re-envuelve.
func TestRenderWithTitleRecortaSinWrap(t *testing.T) {
	largo := strings.Repeat("x", 100)
	out := RenderWithTitle(Rounded(), nil, "", largo, 12)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3 (el recorte no añade líneas)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 12 {
			t.Errorf("línea %d ancho = %d, want 12", i, w)
		}
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 12 {
		t.Errorf("contenido recortado ancho = %d, want 12", got)
	}
}

// El recorte es ANSI-safe: la secuencia de color del contenido no se corrompe.
func TestRenderWithTitleRecorteANSI(t *testing.T) {
	contenido := "\x1b[31m" + strings.Repeat("ab", 40) + "\x1b[0m"
	out := RenderWithTitle(Rounded(), nil, "", contenido, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if w := ansi.StringWidth(lines[1]); w != 10 {
		t.Errorf("ancho ANSI = %d, want 10: %q", w, lines[1])
	}
	if !strings.Contains(lines[1], "\x1b[") {
		t.Errorf("se perdió el ANSI del contenido: %q", lines[1])
	}
}

func TestRenderWithTitleWidthMinimo(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "titulo largo", "x", 1)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 2 {
			t.Errorf("línea %d ancho = %d, want 2 (clamp)", i, w)
		}
	}
}

func TestRenderWithTitleContenidoVacio(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", "", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[1]); got != "│      │" { // interior = 8-2
		t.Errorf("línea interior vacía = %q, want borde + relleno interior", got)
	}
}

// La leyenda inferior se embebe en la línea de borde inferior.
func TestRenderWithTitlesLeyendaInferior(t *testing.T) {
	out := RenderWithTitles(Rounded(), nil, " arriba ", AlignLeft, " abajo ", AlignRight, "c", 24)
	lines := strings.Split(out, "\n")
	bot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(bot, " abajo ") {
		t.Errorf("leyenda inferior ausente: %q", bot)
	}
	if !strings.HasSuffix(bot, "╯") {
		t.Errorf("esquina inferior derecha ausente: %q", bot)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 24 {
			t.Errorf("línea %d ancho = %d, want 24", i, w)
		}
	}
}

// La alineación del título dentro de la línea de borde: lo que decide dónde cae
// el texto entre los dos rellenos. El caso raro es el hueco IMPAR, porque ahí la
// división entera y el reparto se separan (con 3 de hueco, al centro es 1 a la
// izquierda y 2 a la derecha: el resto va a la derecha).
func TestRenderWithTitlesAlineaElTitulo(t *testing.T) {
	// El interior es width-2: con width 9 son 7 celdas, y un título de 4 deja
	// un hueco de 3. Impar a propósito, que es el caso que discrimina.
	const width = 9
	// El borde redondeado: ╭ ╮ ╰ ╯, y el relleno es ─.
	for _, c := range []struct {
		nombre     string
		align      int
		wantTop    string
		wantBottom string
	}{
		{"izquierda: el hueco entero a la derecha", AlignLeft, "╭hola───╮", "╰───────╯"},
		{"centro: 1 a la izquierda y 2 a la derecha (impar)", AlignCenter, "╭─hola──╮", "╰───────╯"},
		{"derecha: el hueco entero a la izquierda", AlignRight, "╭───hola╮", "╰───────╯"},
	} {
		out := RenderWithTitles(Rounded(), nil, "hola", c.align, "", c.align, "x", width)
		lineas := strings.Split(out, "\n")
		if len(lineas) != 3 {
			t.Fatalf("%s: %d líneas, want 3 (borde + contenido + borde):\n%s",
				c.nombre, len(lineas), out)
		}
		if got := lineas[0]; got != c.wantTop {
			t.Errorf("%s: línea superior = %q, want %q", c.nombre, got, c.wantTop)
		}
		if got := lineas[2]; got != c.wantBottom {
			t.Errorf("%s: línea inferior = %q, want %q", c.nombre, got, c.wantBottom)
		}
	}
}

// Las dos líneas de borde son independientes: el título de arriba puede estar a
// un lado y la leyenda de abajo al otro, y eso es justo lo que el panel del log
// hace (título a la izquierda, leyenda a la derecha).
func TestRenderWithTitlesAlineaCadaLineaPorSeparado(t *testing.T) {
	// "T" mide 1 sobre un interior de 7: el hueco es 6.
	out := RenderWithTitles(Rounded(), nil, "T", AlignLeft, "B", AlignRight, "x", 9)
	lineas := strings.Split(out, "\n")
	if got := lineas[0]; got != "╭T──────╮" {
		t.Errorf("línea superior = %q, want ╭T──────╮", got)
	}
	if got := lineas[2]; got != "╰──────B╯" {
		t.Errorf("línea inferior = %q, want ╰──────B╯", got)
	}
}

// Sin título, la línea de borde es todo relleno: no hay nada que alinear, y una
// alineación cualquiera no puede dejar un hueco.
func TestRenderWithTitlesSinTituloNoDejaHueco(t *testing.T) {
	for _, align := range []int{AlignLeft, AlignCenter, AlignRight, 99} {
		out := RenderWithTitles(Rounded(), nil, "", align, "", align, "x", 9)
		lineas := strings.Split(out, "\n")
		if got := lineas[0]; got != "╭───────╮" {
			t.Errorf("align=%d sin título: línea superior = %q, want ╭───────╮", align, got)
		}
		if got := lineas[2]; got != "╰───────╯" {
			t.Errorf("align=%d sin título: línea inferior = %q, want ╰───────╯", align, got)
		}
	}
}
