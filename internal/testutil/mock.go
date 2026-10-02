package testutil

import (
	"fmt"
	"strings"
	"testing"
)

// MockTB es un TB que registra los fallos en vez de matar el proceso.
//
// Existe para cubrir las ramas de error de los helpers de este paquete. Casi
// todas son del mismo tipo: os.MkdirAll, os.WriteFile y cmd.CombinedOutput solo
// fallan cuando el disco o el proceso fallan, y contra un *testing.T de verdad
// no hay forma de llegar a ellas sin que el test que las cubre se mate a sí
// mismo. Con este doble, el mismo fallo se provoca apuntando a un path con un
// fichero donde debería ir un directorio, y el fallo queda anotado.
//
// Helper() y TempDir() son reales: el primero no hace nada (el doble no es un
// test), y el segundo delega en el testing.T que se le pasó, para que los
// temporales sigan bajo el directorio del test que los pidio y se limpien con
// él.

// MockTB registra los fallos de un helper de testutil sin abortar el test.
type MockTB struct {
	// Failures recoge el mensaje de cada Helper/Fatal que se lanzo.
	Failures []string
	// Temp es el testing.T al que se delegan TempDir(). Si es nil, TempDir
	// devuelve "".
	Temp testing.TB
}

// Helper no hace nada: el doble no participa del attribution de lineas de un
// test real, y llamarla haria que el stack trace del helper apuntase aqui.
func (m *MockTB) Helper() {}

// Fatal anota el fallo. No mata el proceso: eso es justo lo que permite seguir
// y comprobar que el helper se detuvo donde deberia.
func (m *MockTB) Fatal(args ...any) {
	m.Failures = append(m.Failures, strings.TrimSpace(fmt.Sprint(args...)))
}

// Fatalf anota el fallo con formato.
func (m *MockTB) Fatalf(format string, args ...any) {
	m.Failures = append(m.Failures, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// TempDir delega en el test real. Sin Temp, no hay directorio temporal que
// devolver y los helpers que lo usan fallarian por otra razon.
func (m *MockTB) TempDir() string {
	if m.Temp == nil {
		return ""
	}
	return m.Temp.TempDir()
}

// Failed reporta si se registro algun fallo.
func (m *MockTB) Failed() bool { return len(m.Failures) > 0 }
