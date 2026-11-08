package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module oci-maintenance-router\n\ngo 1.27.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

func fakeBuild(directory string) ([]entry, error) {
	binary := []byte("binary")
	if err := os.WriteFile(filepath.Join(directory, "func"), binary, 0755); err != nil {
		return nil, err
	}
	return []entry{{"func", binary}, {"resources/notices/fixture/LICENSE", []byte("notice")}}, nil
}

func TestCommandWritesCompleteArtifactsAndPreservesExisting(t *testing.T) {
	repository(t)
	outdir := filepath.Join(t.TempDir(), "package")
	var out bytes.Buffer
	if code := run([]string{outdir}, &out, fakeBuild); code != 0 {
		t.Fatalf("status %d: %s", code, out.String())
	}
	for _, name := range []string{"func", "function.zip", "function-manifest.json", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(outdir, name))
		if err != nil || len(data) == 0 {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	sums, _ := os.ReadFile(filepath.Join(outdir, "SHA256SUMS"))
	for _, name := range []string{"func", "function.zip", "function-manifest.json"} {
		data, _ := os.ReadFile(filepath.Join(outdir, name))
		if !strings.Contains(string(sums), hash(data)+"  "+name+"\n") {
			t.Fatalf("checksum missing: %s", name)
		}
	}
	called := false
	if run([]string{outdir}, &out, func(string) ([]entry, error) { called = true; return nil, nil }) == 0 || called {
		t.Fatal("existing directory accepted")
	}
	after, _ := os.ReadFile(filepath.Join(outdir, "SHA256SUMS"))
	if !bytes.Equal(after, sums) {
		t.Fatal("existing output modified")
	}
}

func TestCommandFailureCleanupAndPaths(t *testing.T) {
	root := repository(t)
	outer := t.TempDir()
	link := filepath.Join(outer, "checkout")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"missing": nil, "extra": {"a", "b"}, "relative": {"relative"},
		"inside checkout": {filepath.Join(root, "build")}, "symlink into checkout": {filepath.Join(link, "build")},
		"missing parent": {filepath.Join(outer, "missing", "output")},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if run(args, &out, func(string) ([]entry, error) { t.Fatal("build attempted"); return nil, nil }) == 0 {
				t.Fatal("invalid output accepted")
			}
		})
	}
	for _, kind := range []string{"build", "archive", "function.zip", "SHA256SUMS"} {
		t.Run(kind, func(t *testing.T) {
			output := filepath.Join(outer, kind)
			var out bytes.Buffer
			code := run([]string{output}, &out, func(dir string) ([]entry, error) {
				_ = os.WriteFile(filepath.Join(dir, "partial"), []byte("partial"), 0600)
				if kind == "build" {
					return nil, errors.New("build failed")
				}
				if kind != "archive" {
					if err := os.Mkdir(filepath.Join(dir, kind), 0700); err != nil {
						t.Fatal(err)
					}
					return fakeBuild(dir)
				}
				return nil, nil
			})
			if code == 0 {
				t.Fatal("failure accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("partial output retained: %v", err)
			}
		})
	}
}

func TestCommandRejectsWrongWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	if run([]string{filepath.Join(t.TempDir(), "package")}, &out, fakeBuild) == 0 {
		t.Fatal("accepted wrong working directory")
	}
}

func TestCommandRejectsCaseAliasInsideCheckout(t *testing.T) {
	root := repository(t)
	alias := strings.ToUpper(root)
	original, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	aliased, err := os.Stat(alias)
	if os.IsNotExist(err) {
		t.Skip("case-sensitive filesystem")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(original, aliased) {
		t.Skip("case-sensitive filesystem")
	}
	var out bytes.Buffer
	code := run([]string{filepath.Join(alias, "output")}, &out, func(string) ([]entry, error) {
		t.Fatal("build attempted inside checkout through case alias")
		return nil, nil
	})
	if code == 0 {
		t.Fatal("case alias accepted")
	}
}
