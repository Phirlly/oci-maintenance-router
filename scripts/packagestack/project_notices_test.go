package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBundlePreservesProjectNotices(t *testing.T) {
	root, input := inputFixture(t)
	data, err := bundle(root, input)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LICENSE", "NOTICE"} {
		original, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		f, err := reader.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		closeErr := f.Close()
		if err != nil || closeErr != nil || !bytes.Equal(got, original) {
			t.Fatalf("project notice changed: %s", name)
		}
	}
}

func TestBundleRejectsMissingOrEmptyProjectNotices(t *testing.T) {
	for _, name := range []string{"LICENSE", "NOTICE"} {
		for _, missing := range []bool{true, false} {
			t.Run(name+map[bool]string{true: " missing", false: " empty"}[missing], func(t *testing.T) {
				root, input := localStackFixture(t)
				p := filepath.Join(root, name)
				var err error
				if missing {
					err = os.Remove(p)
				} else {
					err = os.WriteFile(p, nil, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := bundle(root, input); err == nil {
					t.Fatal("accepted a project without complete legal files")
				}
			})
		}
	}
}

func TestBundleRejectsMissingOrDifferentFunctionNotices(t *testing.T) {
	for _, name := range []string{"LICENSE", "NOTICE"} {
		for _, missing := range []bool{true, false} {
			t.Run(name+map[bool]string{true: " missing", false: " different"}[missing], func(t *testing.T) {
				root, _ := inputFixture(t)
				legal := map[string][]byte{}
				for _, n := range []string{"LICENSE", "NOTICE"} {
					data, err := os.ReadFile(filepath.Join(root, n))
					if err != nil {
						t.Fatal(err)
					}
					legal[n] = data
				}
				if missing {
					delete(legal, name)
				} else {
					legal[name] = []byte("outdated project notice")
				}
				input := functionInputFixture(t, legal)
				if _, err := bundle(root, input); err == nil {
					t.Fatal("accepted a valid but stale Function archive")
				}
			})
		}
	}
}
