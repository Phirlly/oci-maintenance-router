package main

import (
	"encoding/json"
	"testing"
)

func TestGoCommandControlsBuildEnvironment(t *testing.T) {
	for key, value := range map[string]string{"GOFLAGS": "-race", "GOWORK": "/not/a/workspace", "GOOS": "darwin", "GOARCH": "arm64", "GOAMD64": "v4", "CGO_ENABLED": "1", "GOEXPERIMENT": "invalid", "GOFIPS140": "invalid", "GOROOT": "/not/a/toolchain"} {
		t.Setenv(key, value)
	}
	got, err := executeGo("env", "-json", "GOENV", "GOWORK", "GOTOOLCHAIN", "GOFLAGS", "GOOS", "GOARCH", "GOAMD64", "CGO_ENABLED", "GOEXPERIMENT", "GOFIPS140", "GOVERSION")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(got, &values); err != nil {
		t.Fatal(err)
	}
	// Disabled GOENV is reported as an empty configuration-file path by go env.
	for key, want := range map[string]string{"GOENV": "", "GOWORK": "off", "GOTOOLCHAIN": "go1.27.4", "GOFLAGS": "", "GOOS": "linux", "GOARCH": "amd64", "GOAMD64": "v1", "CGO_ENABLED": "0", "GOEXPERIMENT": "", "GOFIPS140": "off", "GOVERSION": "go1.27.4"} {
		if values[key] != want {
			t.Fatalf("%s=%q want %q", key, values[key], want)
		}
	}
}

func TestGoCommandFailsWhenExecutableUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got, err := executeGo("version")
	if err == nil || got != nil {
		t.Fatal("missing compiler accepted")
	}
}
