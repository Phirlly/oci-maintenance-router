package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandWritesLocalArtifactsAndPreservesExisting(t *testing.T) {
	root := repository(t)
	parent := filepath.Join(root, ".local", "artifacts")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(parent, "function")
	var output bytes.Buffer
	if status := run([]string{directory}, &output, fakeBuild); status != 0 {
		t.Fatalf("local output rejected: %s", &output)
	}
	archive := filepath.Join(directory, "function.zip")
	before, err := os.ReadFile(archive)
	if err != nil || len(before) == 0 {
		t.Fatalf("missing archive: %v", err)
	}
	if run([]string{directory}, &output, func(string) ([]entry, error) {
		t.Fatal("build attempted over existing artifacts")
		return nil, nil
	}) == 0 {
		t.Fatal("existing local output accepted")
	}
	after, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing local archive changed")
	}
}

func TestLocalOutputDoesNotPermitSourcePaths(t *testing.T) {
	for _, scenario := range []string{"sibling", "parent traversal", "nested symlink", "symlinked local"} {
		t.Run(scenario, func(t *testing.T) {
			root := repository(t)
			local := filepath.Join(root, ".local")
			source := filepath.Join(root, "source")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			var parent string
			if scenario == "symlinked local" {
				if err := os.Symlink(source, local); err != nil {
					t.Fatal(err)
				}
				parent = local
			} else {
				if err := os.Mkdir(local, 0700); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "sibling":
					parent = filepath.Join(root, ".local-other")
					if err := os.Mkdir(parent, 0700); err != nil {
						t.Fatal(err)
					}
				case "parent traversal":
					parent = local + string(filepath.Separator) + ".."
				case "nested symlink":
					parent = filepath.Join(local, "escape")
					if err := os.Symlink(source, parent); err != nil {
						t.Fatal(err)
					}
				}
			}
			requested := parent + string(filepath.Separator) + "output"
			var output bytes.Buffer
			if run([]string{requested}, &output, func(string) ([]entry, error) {
				t.Fatal("build attempted in source directory")
				return nil, nil
			}) == 0 {
				t.Fatal("source output accepted")
			}
			if _, err := os.Lstat(requested); !os.IsNotExist(err) {
				t.Fatalf("rejected output was created: %v", err)
			}
		})
	}
}
