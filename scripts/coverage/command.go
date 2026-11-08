package main

import (
	"io"
	"os/exec"
)

func runTests(profile string, output io.Writer) error {
	command := exec.Command("go", "test", "./...", "-count=1", "-timeout=30s",
		"-coverpkg=./...", "-coverprofile="+profile)
	command.Stdout, command.Stderr = output, output
	return command.Run()
}
