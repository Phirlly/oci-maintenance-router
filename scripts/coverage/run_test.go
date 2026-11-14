package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("go.mod", []byte("module oci-maintenance-router\n\ngo 1.27.4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestGateStatusAndTemporaryProfile(t *testing.T) {
	root := repository(t)
	for _, tc := range []struct {
		name, profile string
		status        int
	}{
		{"at floor", "mode: set\na.go:1.1,2.1 97 1\nb.go:1.1,2.1 3 0\n", 0},
		{"below floor", "mode: set\na.go:1.1,2.1 9696 1\nb.go:1.1,2.1 304 0\n", 1},
		{"empty", "", 1},
		{"malformed", "mode: set\nbroken", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			var path string
			status := run(nil, &output, func(profile string, out io.Writer) error {
				path = profile
				if filepath.Dir(path) == root {
					t.Fatal("report written in repository")
				}
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("report was not fresh: %v", err)
				}
				fmtOutput := "test output\n"
				io.WriteString(out, fmtOutput)
				return os.WriteFile(path, []byte(tc.profile), 0600)
			})
			if status != tc.status {
				t.Fatalf("status=%d want=%d output=%s", status, tc.status, output.String())
			}
			if path == "" {
				t.Fatal("test runner was not invoked")
			}
			if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary report not cleaned: %v", err)
			}
			if !strings.Contains(output.String(), "test output") {
				t.Fatal("test output lost")
			}
			if tc.status == 0 && !strings.Contains(output.String(), "PASS") {
				t.Fatalf("missing successful result: %s", output.String())
			}
			if tc.status != 0 && strings.Contains(output.String(), "PASS") {
				t.Fatalf("failure reported success: %s", output.String())
			}
		})
	}
}

func TestFailedTestsCannotPassWithValidProfile(t *testing.T) {
	repository(t)
	var output bytes.Buffer
	var path string
	status := run(nil, &output, func(profile string, _ io.Writer) error {
		path = profile
		if err := os.WriteFile(profile, []byte("mode: set\na.go:1.1,2.1 100 1\n"), 0600); err != nil {
			t.Fatal(err)
		}
		return errors.New("synthetic test failure")
	})
	if status == 0 || !strings.Contains(output.String(), "tests failed") || strings.Contains(output.String(), "PASS") {
		t.Fatalf("status=%d output=%s", status, output.String())
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary report not cleaned: %v", err)
	}
}

func TestMissingReportFails(t *testing.T) {
	repository(t)
	var output bytes.Buffer
	if status := run(nil, &output, func(string, io.Writer) error { return nil }); status == 0 {
		t.Fatalf("missing report passed: %s", output.String())
	}
}

func TestGateRejectsArgumentsAndWrongDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{nil, {"--minimum=1"}} {
		var output bytes.Buffer
		status := run(args, &output, func(string, io.Writer) error { t.Fatal("runner invoked for invalid command"); return nil })
		if status == 0 {
			t.Fatal("invalid command passed")
		}
	}
	if err := os.WriteFile("go.mod", []byte("module unrelated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if status := run(nil, io.Discard, func(string, io.Writer) error { t.Fatal("runner invoked in wrong module"); return nil }); status == 0 {
		t.Fatal("wrong module accepted")
	}
}

func TestTemporaryDirectoryFailure(t *testing.T) {
	repository(t)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	var output bytes.Buffer
	if status := run(nil, &output, func(string, io.Writer) error { t.Fatal("runner invoked without report directory"); return nil }); status == 0 {
		t.Fatal("temporary directory failure passed")
	}
}
