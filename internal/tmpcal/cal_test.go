package tmpcal

import "testing"

func TestEsPositivo(t *testing.T) {
	if !EsPositivo(1) {
		t.Error("1 debe ser positivo")
	}
	if EsPositivo(0) {
		t.Error("0 no es positivo")
	}
	if EsPositivo(-1) {
		t.Error("-1 no es positivo")
	}
}
