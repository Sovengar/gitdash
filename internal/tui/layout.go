// Presupuesto de alto del dashboard: calcula qué secciones se ven y cuánto
// alto queda para el cuerpo central (tabla o detalle) y para el panel de
// preview del repo bajo el cursor. Fuente única compartida por el render, el
// detalle y los tests.
package tui

const (
	statsSectionLines  = 3 // 2 bordes + 1 línea de contenido
	filterSectionLines = 3 // 2 bordes + 1 línea de contenido
	keybindsChrome     = 2 // solo los bordes (las hints van dentro)
	tableChrome        = 3 // 2 bordes + cabecera de columnas
	defaultHintLines   = 3

	previewChrome = 2 // solo los bordes (la ficha va dentro)
	// previewShare es la fracción del alto LIBRE que se lleva el panel (2/5 =
	// 40%), el mismo reparto que usa prdash para su ficha de ítem.
	previewShare = 2
	// minPreviewLines es el alto que merece la ficha: la cabecera de estado más
	// una lista con su cabecera y la ayuda al pie. Por debajo de la cabecera
	// (detailHeadLines) el panel no dice nada, así que entonces no entra.
	minPreviewLines = 10
	// minBodyLines son las filas que la tabla conserva para que el panel entre.
	// Por debajo no hay ventana que desplazar.
	minBodyLines = 3
)

// layout describe las secciones visibles y el alto reservado a cada cuerpo.
type layout struct {
	showStats    bool
	showFilter   bool
	showKeybinds bool
	hintLines    int
	bodyLines    int
	// previewLines es el alto de contenido del panel con la ficha del repo bajo
	// el cursor; 0 = panel oculto.
	previewLines int
}

// computeLayout reparte el alto de la terminal entre las secciones del dashboard.
//
// El panel de preview es ADITIVO: entra con su share del alto libre (el que no
// ocupan las secciones de alto fijo) y solo se queda si no le cuesta nada a lo
// que ya había. Concretamente se busca el mayor alto de panel que (a) deje
// minBodyLines filas de tabla y (b) no obligue a recortar hints ni a ocultar
// stats o keybinds. Si no hay ningún alto que cumpla las dos, el panel no se
// dibuja: media ficha por perder media pantalla es peor que no tener panel.
//
// Sin panel el reparto es el de siempre (hints → 0, keybinds fuera, stats
// fuera), así que en terminales bajas esto no cambia nada de lo que se veía
// antes. keepKeybinds impide degradar keybinds: con un aviso armado es la
// única fuente de las teclas que espera la app.
func computeLayout(height int, hasFilter bool, keybindsLines int, keepKeybinds bool) layout {
	filterH := 0
	if hasFilter {
		filterH = filterSectionLines
	}
	chrome := tableChrome

	sinPanel := fitLayout(height, chrome, filterH, 0, keybindsLines, keepKeybinds)
	lay := sinPanel
	for preview := panelHeight(height, chrome, filterH, keybindsLines); preview >= detailHeadLines; preview-- {
		l := fitLayout(height, chrome, filterH, preview, keybindsLines, keepKeybinds)
		if l.bodyLines >= minBodyLines && l.mismaChromeQue(sinPanel) {
			l.previewLines = preview
			lay = l
			break
		}
	}
	// El filtro es la única sección cuya visibilidad no se degrada: se pinta
	// mientras se esté escribiendo o confirmado, y su alto ya está contado.
	lay.showFilter = hasFilter
	return lay
}

// panelHeight es el alto de contenido que le corresponde al panel por su share
// del alto libre, con su mínimo y sin pasarse del hueco que hay. Es el punto de
// partida de la búsqueda, no una decisión final: computeLayout lo va bajando
// hasta que el resto del dashboard lo permita.
func panelHeight(height, chrome, filterH, keybinds int) int {
	free := max(0, height-(chrome+filterH+statsSectionLines+keybindsChrome+max(0, keybinds)+previewChrome))
	return min(max(minPreviewLines, free*previewShare/5), free)
}

// fitLayout reparte el alto con un panel de altura fija, degradando en el orden
// de siempre: hints de keybinds (hasta 0), keybinds fuera, stats fuera. El
// cuerpo central nunca baja de una línea; el mínimo de filas que el panel tiene
// que respetar (minBodyLines) lo decide quien lo elige, no este reparto.
func fitLayout(height, chrome, filterH, preview, keybindsLines int, keepKeybinds bool) layout {
	showStats := true
	showKeybinds := true
	hint := max(0, keybindsLines)

	// reserved es todo lo que no es cuerpo central.
	reserved := func() int {
		n := chrome + filterH
		if showStats {
			n += statsSectionLines
		}
		if showKeybinds {
			n += keybindsChrome + hint
		}
		if preview > 0 {
			n += previewChrome + preview
		}
		return n
	}
	if !keepKeybinds {
		for hint > 0 && reserved()+1 > height {
			hint--
		}
	}
	if hint == 0 {
		showKeybinds = false
	}
	if reserved()+1 > height {
		showStats = false
	}

	return layout{
		showStats:    showStats,
		showKeybinds: showKeybinds,
		hintLines:    hint,
		bodyLines:    max(1, height-reserved()),
	}
}

// mismaChromeQue dice si dos repartos coinciden en las secciones que existían
// antes del panel. Es la condición de aditividad: si el panel obliga a recortar
// una hint o a ocultar stats/keybinds, no entra.
func (l layout) mismaChromeQue(o layout) bool {
	return l.showStats == o.showStats &&
		l.showKeybinds == o.showKeybinds &&
		l.hintLines == o.hintLines
}
