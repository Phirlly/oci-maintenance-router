package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const compilerVersion = "go1.27.3"

type environment struct{ GOROOT, GOMODCACHE, GOVERSION string }
type goRunner func(...string) ([]byte, error)

// executeGo controls build-affecting settings without changing the user's setup.
func executeGo(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...)
	command.WaitDelay = 2 * time.Second
	command.Env = append(os.Environ(), "GOENV=off", "GOWORK=off", "GOTOOLCHAIN="+compilerVersion,
		"GOROOT=", "GOFLAGS=", "GOEXPERIMENT=", "GOFIPS140=off",
		"CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w", args[0], err)
	}
	return output, nil
}

func buildFunction(directory string, runGo goRunner) ([]entry, error) {
	data, err := runGo("env", "-json", "GOROOT", "GOMODCACHE", "GOVERSION")
	if err != nil {
		return nil, err
	}
	var env environment
	if json.Unmarshal(data, &env) != nil || env.GOVERSION != compilerVersion ||
		!filepath.IsAbs(env.GOROOT) || !filepath.IsAbs(env.GOMODCACHE) {
		return nil, fmt.Errorf("packaging requires %s and absolute toolchain/cache paths", compilerVersion)
	}
	if _, err := runGo("mod", "verify"); err != nil {
		return nil, err
	}
	target := filepath.Join(directory, "func")
	if _, err := runGo("build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-pgo=off", "-ldflags=-w", "-o", target, "./cmd/router"); err != nil {
		return nil, err
	}
	binary, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read built executable: %w", err)
	}
	if err := inspectBinary(binary); err != nil {
		return nil, err
	}
	notices, err := collectNotices(env)
	if err != nil {
		return nil, err
	}
	return append([]entry{{"func", binary}}, notices...), nil
}
