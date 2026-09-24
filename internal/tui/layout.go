// Presupuesto de alto del dashboard: calcula qué secciones se ven y cuánto
// alto queda para el cuerpo central (tabla o detalle). Fuente única compartida
// por el render, el detalle y los tests.
package tui

const (
	statsSectionLines  = 3 // 2 bordes + 1 línea de contenido
	filterSectionLines = 3 // 2 bordes + 1 línea de contenido
	keybindsChrome     = 2 // solo los bordes (las hints van dentro)
	tableChrome        = 3 // 2 bordes + cabecera de columnas
	detailChrome       = 2 // 2 bordes (el detalle no tiene cabecera de tabla)
	defaultHintLines   = 3
)

// layout describe las secciones visibles y el alto reservado al cuerpo.
type layout struct {
	showStats    bool
	showFilter   bool
	showKeybinds bool
	hintLines    int
	bodyLines    int
}

// computeLayout reparte el alto de la terminal entre las secciones. En
// terminales bajas degrada en orden: recortar hints de keybinds (hasta
// defaultHintLines→0), ocultar keybinds, ocultar stats y, por último,
// garantizar bodyLines >= 1. hintBarLines es el número real de líneas de hints
// (config): la reserva de keybinds nunca pide más de las que se van a pintar.
// Seguro con height = 0 (primer render antes de WindowSizeMsg).
func computeLayout(height int, hasFilter, detailOpen bool, hintBarLines int) layout {
	filterH := 0
	if hasFilter {
		filterH = filterSectionLines
	}
	chrome := tableChrome
	if detailOpen {
		chrome = detailChrome
	}

	showStats := true
	showKeybinds := true
	hint := min(defaultHintLines, max(0, hintBarLines))

	fixed := func() int {
		n := chrome + filterH
		if showStats {
			n += statsSectionLines
		}
		if showKeybinds {
			n += keybindsChrome + hint
		}
		return n
	}

	for hint > 0 && height-fixed() < 1 {
		hint--
	}
	if hint == 0 {
		showKeybinds = false
	}
	if height-fixed() < 1 {
		showStats = false
	}

	return layout{
		showStats:    showStats,
		showFilter:   hasFilter,
		showKeybinds: showKeybinds,
		hintLines:    hint,
		bodyLines:    max(1, height-fixed()),
	}
}
