package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sum(data []byte) string { d := sha256.Sum256(data); return hex.EncodeToString(d[:]) }

func inputFixture(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	entries := []map[string]any{}
	for _, name := range []string{"func", "resources/notices/LICENSE"} {
		content := []byte("synthetic " + name)
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0644)
		if name == "func" {
			h.SetMode(0755)
		}
		file, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(content); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, map[string]any{"name": name, "sha256": sum(content), "bytes": len(content)})
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data.Bytes())
	manifest := map[string]any{"schemaVersion": 1, "purpose": "maintenance-router-runtime", "architecture": "X86", "operatingSystem": "linux", "goVersion": "go1.27.4",
		"archiveSha256": sum(data.Bytes()), "archiveSha256Base64": base64.StdEncoding.EncodeToString(hash[:]), "archiveBytes": data.Len(),
		"binarySha256": sum([]byte("synthetic func")), "entries": entries}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"function.zip": data.Bytes(), "function-manifest.json": encoded} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, dir
}

func TestBundleContainsOnlyDeploymentInputs(t *testing.T) {
	root, dir := inputFixture(t)
	got, err := bundle(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatalf("not a valid RM ZIP: %v", err)
	}
	names := map[string]bool{}
	for _, f := range reader.File {
		names[f.Name] = true
		if strings.Contains(f.Name, "test") || strings.HasPrefix(f.Name, "docs/") || strings.HasPrefix(f.Name, ".git") {
			t.Fatalf("private/development content %s", f.Name)
		}
	}
	for _, name := range []string{"schema.yaml", "versions.tf", ".terraform.lock.hcl", "modules/configuration/render.tf", "function.zip", "function-manifest.json", "release.json", "variables_operation.tf", "identity_diagnostic.tf", "operator.tf", "variables_operator.tf", "identity_operator.tf"} {
		if !names[name] {
			t.Errorf("missing %s", name)
		}
	}
	again, err := bundle(root, dir)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatal("bundle is not deterministic")
	}
}

func TestBundleRejectsCorruptArchiveOrManifest(t *testing.T) {
	for _, kind := range []string{"zip", "hash", "architecture", "inventory", "missing", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, dir := inputFixture(t)
			p := filepath.Join(dir, "function-manifest.json")
			if kind == "zip" {
				p = filepath.Join(dir, "function.zip")
			}
			if kind == "missing" {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			} else if kind == "symlink" {
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(t.TempDir(), "manifest")
				if err := os.WriteFile(other, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, p); err != nil {
					t.Fatal(err)
				}
			} else if kind == "zip" {
				if err := os.WriteFile(p, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				var m map[string]any
				if json.Unmarshal(data, &m) != nil {
					t.Fatal("fixture")
				}
				switch kind {
				case "hash":
					m["archiveSha256"] = "bad"
				case "architecture":
					m["architecture"] = "ARM"
				case "inventory":
					m["entries"] = []any{}
				}
				data, err = json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := bundle(root, dir); err == nil {
				t.Fatal("accepted corrupt release input")
			}
		})
	}
}

func TestBundledArchiveBytesAreUnchanged(t *testing.T) {
	root, dir := inputFixture(t)
	got, err := bundle(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(dir, "function.zip"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := reader.Open("function.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	saved, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(original, saved) {
		t.Fatal("Function bytes changed")
	}
}
