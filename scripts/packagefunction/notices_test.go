package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noticeFixture(t *testing.T) environment {
	t.Helper()
	env := environment{GOROOT: t.TempDir(), GOMODCACHE: t.TempDir(), GOVERSION: "go1.27.4"}
	paths := map[string][]string{
		"github.com/fnproject/fdk-go@v0.1.26":                         {"LICENSE", "NOTICE.txt", "THIRD_PARTY_LICENSES.txt"},
		"github.com/gofrs/flock@v0.10.0":                              {"LICENSE"},
		"github.com/oracle/oci-go-sdk/v65@v65.129.0":                  {"LICENSE.txt", "NOTICE.txt", "THIRD_PARTY_LICENSES.txt"},
		"github.com/sony/gobreaker/v2@v2.4.0":                         {"LICENSE"},
		"github.com/youmark/pkcs8@v0.0.0-20240726163527-a2c0da244d78": {"LICENSE"},
		"golang.org/x/crypto@v0.56.0":                                 {"LICENSE"},
	}
	for directory, files := range paths {
		for _, name := range files {
			path := filepath.Join(env.GOMODCACHE, directory, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("notice "+directory+"/"+name), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(env.GOROOT, "LICENSE"), []byte("Go license"), 0600); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestNoticesUseOnlyProductionAllowlist(t *testing.T) {
	env := noticeFixture(t)
	extra := filepath.Join(env.GOMODCACHE, "github.com/oracle/oci-go-sdk/v65@v65.129.0", "THIRD_PARTY_LICENSES_DEV.txt")
	if err := os.WriteFile(extra, []byte("development only"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := collectNotices(env)
	if err != nil || len(got) != 11 {
		t.Fatalf("notices %d: %v", len(got), err)
	}
	seen := map[string]bool{}
	for _, e := range got {
		if seen[e.name] || !strings.HasPrefix(e.name, "resources/notices/") ||
			strings.Contains(e.name, "_DEV") || len(e.data) == 0 {
			t.Fatalf("unexpected notice %q", e.name)
		}
		seen[e.name] = true
		source := filepath.Join(env.GOMODCACHE, strings.TrimPrefix(e.name, "resources/notices/"))
		if strings.HasPrefix(e.name, "resources/notices/go1.27.4/") {
			source = filepath.Join(env.GOROOT, "LICENSE")
		}
		original, err := os.ReadFile(source)
		if err != nil || !bytes.Equal(e.data, original) {
			t.Fatalf("changed upstream notice %s", e.name)
		}
	}
}

func TestNoticesFailOnMissingOrEmptyFiles(t *testing.T) {
	for _, name := range []string{"module missing", "module empty", "Go missing", "Go empty"} {
		t.Run(name, func(t *testing.T) {
			env := noticeFixture(t)
			path := filepath.Join(env.GOMODCACHE, "github.com/fnproject/fdk-go@v0.1.26", "LICENSE")
			if strings.HasPrefix(name, "Go") {
				path = filepath.Join(env.GOROOT, "LICENSE")
			}
			var err error
			if strings.HasSuffix(name, "missing") {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, nil, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := collectNotices(env)
			if err == nil || got != nil {
				t.Fatal("incomplete notices accepted")
			}
		})
	}
}
