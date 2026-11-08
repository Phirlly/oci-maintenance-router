package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandWritesNewExternalBundleAndProtectsExistingOutput(t *testing.T) {
	root, dir := inputFixture(t)
	t.Chdir(root)
	path := filepath.Join(t.TempDir(), "stack.zip")
	var output bytes.Buffer
	if status := run([]string{dir, path}, &output); status != 0 || !strings.Contains(output.String(), "PREVIEW-only RM bundle:") {
		t.Fatalf("%d %s", status, &output)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(original) == 0 {
		t.Fatal("empty bundle")
	}
	if status := run([]string{dir, path}, &output); status != 1 {
		t.Fatal("overwrote existing output")
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, saved) {
		t.Fatal("existing file changed")
	}
}

func TestCommandRejectsInvalidLocationsAndInputs(t *testing.T) {
	for _, kind := range []string{"arguments", "relative input", "relative output", "checkout", "missing parent", "invalid input", "wrong root", "output symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, dir := inputFixture(t)
			t.Chdir(root)
			path := filepath.Join(t.TempDir(), "stack.zip")
			args := []string{dir, path}
			expected := 1
			switch kind {
			case "arguments":
				args = nil
				expected = 2
			case "relative input":
				args[0] = "relative"
				expected = 2
			case "relative output":
				args[1] = "relative"
				expected = 2
			case "checkout":
				args[1] = filepath.Join(root, "should-not-exist.zip")
			case "missing parent":
				args[1] = filepath.Join(t.TempDir(), "missing", "stack.zip")
			case "invalid input":
				args[0] = t.TempDir()
			case "wrong root":
				t.Chdir(t.TempDir())
			case "output symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			if status := run(args, &output); status != expected {
				t.Fatalf("%s: %d %s", kind, status, &output)
			}
		})
	}
}

func TestOutputDirectoryRejectsCheckoutAliases(t *testing.T) {
	root, _ := inputFixture(t)
	alias := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := outsideCheckout(root, filepath.Join(alias, "bad.zip")); err == nil {
		t.Fatal("accepted checkout symlink")
	}
	if _, err := outsideCheckout(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("accepted missing root")
	}
}

func TestReadInputRejectsOversizedOrNonregularFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "input")
	if err := os.WriteFile(p, []byte("four"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInput(p, 3); err == nil {
		t.Fatal("size bound ignored")
	}
	if _, err := readInput(dir, 100); err == nil {
		t.Fatal("directory accepted")
	}
}
