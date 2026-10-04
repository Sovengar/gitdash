package main

import (
	"fmt"
	"os"
	"os/exec"
)

func covdataToText(dir string) error {
	bin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go is not in PATH: %w", err)
	}
	out := dir + "/cov.out"
	cmd := exec.Command(bin, "tool", "covdata", "textfmt", "-i="+dir, "-o="+out)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("covdata textfmt: %w", err)
	}
	return nil
}
