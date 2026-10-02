package main

import (
	"fmt"
	"os"
	"os/exec"
)

// covdata convierte los contadores en binario que deja un binario construido con
// -cover en un perfil de texto, que es lo unico que se puede leer.
func covdataToText(dir string) error {
	bin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go no esta en el PATH: %w", err)
	}
	out := dir + "/cov.out"
	cmd := exec.Command(bin, "tool", "covdata", "textfmt", "-i="+dir, "-o="+out)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("covdata textfmt: %w", err)
	}
	return nil
}
