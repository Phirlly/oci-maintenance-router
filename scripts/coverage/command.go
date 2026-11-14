package main

import (
	"io"
	"os/exec"
)

func runTests(profile string, output io.Writer) error {
	// The suite includes a real Function build with a two-minute command budget.
	command := exec.Command("go", "test", "./...", "-count=1", "-timeout=3m",
		"-coverpkg=./...", "-coverprofile="+profile)
	command.Stdout, command.Stderr = output, output
	return command.Run()
}
