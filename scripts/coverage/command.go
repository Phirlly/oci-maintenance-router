package main

import (
	"io"
	"os/exec"
)

func runTests(profile string, output io.Writer) error {
	// The suite includes a real Function build with a two-minute command budget.
	// Match import paths so project-local toolchains and caches are not instrumented.
	command := exec.Command("go", "test", "./...", "-count=1", "-timeout=3m",
		"-coverpkg=oci-maintenance-router/...", "-coverprofile="+profile)
	command.Stdout, command.Stderr = output, output
	return command.Run()
}
