package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandWritesLocalBundleAndPreservesExisting(t *testing.T) {
	root, input := localStackFixture(t)
	t.Chdir(root)
	parent := filepath.Join(root, ".local", "artifacts")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "stack.zip")
	var output bytes.Buffer
	if status := run([]string{input, path}, &output); status != 0 {
		t.Fatalf("local output rejected: %s", &output)
	}
	before, err := os.ReadFile(path)
	if err != nil || len(before) == 0 {
		t.Fatalf("missing archive: %v", err)
	}
	if run([]string{input, path}, &output) == 0 {
		t.Fatal("existing local output accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing local archive changed")
	}
}

func TestLocalOutputDoesNotPermitSourcePaths(t *testing.T) {
	for _, scenario := range []string{"sibling", "parent traversal", "nested symlink", "symlinked local"} {
		t.Run(scenario, func(t *testing.T) {
			root, input := localStackFixture(t)
			t.Chdir(root)
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
			requested := parent + string(filepath.Separator) + "stack.zip"
			var output bytes.Buffer
			if run([]string{input, requested}, &output) == 0 {
				t.Fatal("source output accepted")
			}
			if _, err := os.Lstat(requested); !os.IsNotExist(err) {
				t.Fatalf("rejected output was created: %v", err)
			}
		})
	}
}

// Keep writable location tests isolated from the checkout that supplies inputs.
func localStackFixture(t *testing.T) (string, string) {
	t.Helper()
	source, input := inputFixture(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module oci-maintenance-router\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LICENSE", "NOTICE"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range stackFiles {
		data, err := os.ReadFile(filepath.Join(source, "infra", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "infra", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, input
}
