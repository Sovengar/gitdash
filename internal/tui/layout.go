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
	// una lista con su cabecera. Por debajo de la cabecera (detailHeadLines) el
	// panel no dice nada, así que entonces no entra.
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
// formMin > 0 dice que el cuerpo lo ocupa un formulario (el overlay de PR) que
// necesita al menos eso: con el formulario abierto la búsqueda de panel se
// salta entera, porque un panel de preview competiría por el alto de algo que
// el usuario está escribiendo y la ficha del repo bajo el cursor ya no describe
// nada —con el teclado capturado, el cursor puede estar en cualquier sitio—.
//
// Sin panel el reparto es el de siempre (hints → 0, keybinds fuera, stats
// fuera), así que en terminales bajas esto no cambia nada de lo que se veía
// antes. keepKeybinds impide degradar keybinds: con un aviso armado es la
// única fuente de las teclas que espera la app.
func computeLayout(height int, hasFilter bool, keybindsLines int, keepKeybinds bool, formMin int) layout {
	filterH := 0
	if hasFilter {
		filterH = filterSectionLines
	}
	chrome := tableChrome

	sinPanel := fitLayout(height, chrome, filterH, 0, keybindsLines, keepKeybinds)
	lay := sinPanel
	if formMin <= 0 {
		for _, preview := range panelCandidates(height, chrome, filterH, keybindsLines) {
			l := fitLayout(height, chrome, filterH, preview, keybindsLines, keepKeybinds)
			if l.bodyLines >= minBodyLines && l.mismaChromeQue(sinPanel) {
				l.previewLines = preview
				lay = l
				break
			}
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

// panelCandidates son los altos de panel a probar, del share del hueco libre
// hacia el suelo del panel, ambos extremos incluidos.
//
// Vive aparte y devuelve la lista porque el bucle original `for preview :=
// panelHeight(...); preview >= detailHeadLines; preview--` NO SE PUEDE MATAR: al
// invertir `preview--` en `preview++` la condición sigue siendo cierta desde la
// primera vuelta —el suelo no depende de preview—, así que el mutante es un
// bucle infinito. Y un bucle infinito no lo mata ningún test: el test no
// termina, y gremlins lo reporta como TIMED OUT, un estado que ni entra en
// mutants_total ni ve el gate de CI. O sea: el mutante se escapa del gate por
// la vía del cuelgue, que es la peor forma de escaparse.
//
// El rango se cuenta hacia abajo desde `top` CON UN LÍMITE DE VUELTAS, no
// comparando contra el suelo: `for i := 0; i <= alto; i++` con `preview := top-i`
// dentro recorre exactamente los mismos valores (top, top-1, …, detailHeadLines)
// y su condición no depende de un contador que se pueda invertir hacia arriba. Un
// `i++` en `>=` sale del bucle al segundo elemento y devuelve una lista corta —un bug
// real, con su aserción— en vez de colgar el run entero.
//
// La búsqueda no cambia: computeLayout sigue cogiendo el primer elemento de la
// lista que cumple sus dos condiciones, y la lista está en orden descendente.
func panelCandidates(height, chrome, filterH, keybinds int) []int {
	top := panelHeight(height, chrome, filterH, keybinds)
	if top < detailHeadLines {
		return nil
	}
	vueltas := top - detailHeadLines + 1
	// El recorrido va con `range` sobre el número de vueltas, no con un contador
	// en la cabecera del for. Es la diferencia entre un bucle cuyo mutante
	// INCREMENT_DECREMENT es "devuelve la lista al revés" y uno cuyo mutante es
	// "se cuelga": range no tiene expresión que invertir, así que el mutante de
	// la cabecera desaparece entero en vez de convertirse en un cuelgue.
	//
	// El valor sale de `top-i` con i en orden ASCENDENTE, que es la forma segura:
	// un `i--` aquí tocaría el índice de la lista, no su condición de salida.
	c := make([]int, 0, vueltas)
	for i := range vueltas {
		c = append(c, top-i)
	}
	return c
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
		// Se degrada uno a uno mientras no quepan, en vez de `for hint > 0 &&
		// reserved()+1 > height { hint-- }`. El `hint--` invertido no es un
		// cuelgue —reserved() baja al quitar un hint, así que la guarda se
		// cumple y el bucle sale—, pero el recorrido al revés degrada hints de
		// más y el layout sale con el alto equivocado sin que nada falle.
		//
		// Con `range` no hay expresión post que invertir: el bucle da exactamente
		// tantas vueltas como hints había, y es la guarda la que dice hasta
		// dónde. El `break` es el que decide, no el contador.
		for range hint {
			if reserved()+1 <= height {
				break
			}
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
