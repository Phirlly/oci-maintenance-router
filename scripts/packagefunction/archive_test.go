package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/rand"
	"slices"
	"testing"
)

func hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func TestArchiveInventoryIntegrityAndReproducibility(t *testing.T) {
	entries := []entry{{"resources/notices/fixture/LICENSE", []byte("license")}, {"func", []byte("binary")}}
	packed, manifest, err := archive(entries)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil {
		t.Fatalf("missing valid archive: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("inventory: %d", len(reader.File))
	}
	for i, name := range []string{"func", "resources/notices/fixture/LICENSE"} {
		f := reader.File[i]
		want := []byte("license")
		mode := uint32(0644)
		if i == 0 {
			want = []byte("binary")
			mode = 0755
		}
		if f.Name != name || uint32(f.Mode().Perm()) != mode || f.Modified.Year() != 1980 {
			t.Fatalf("header: %+v", f.FileHeader)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		r.Close()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("entry mismatch %s", name)
		}
	}
	var report struct {
		SchemaVersion       int    `json:"schemaVersion"`
		Purpose             string `json:"purpose"`
		GoVersion           string `json:"goVersion"`
		Architecture        string `json:"architecture"`
		ArchiveSHA256       string `json:"archiveSha256"`
		ArchiveSHA256Base64 string `json:"archiveSha256Base64"`
		BinarySHA256        string `json:"binarySha256"`
		ArchiveBytes        int    `json:"archiveBytes"`
		Entries             []struct {
			Name, SHA256 string
			Bytes        int
		} `json:"entries"`
	}
	if json.Unmarshal(manifest, &report) != nil || report.SchemaVersion != 1 ||
		report.Purpose != "maintenance-router-runtime" || report.GoVersion != "go1.27.4" || report.Architecture != "X86" ||
		report.ArchiveSHA256 != hash(packed) || report.BinarySHA256 != hash([]byte("binary")) ||
		report.ArchiveBytes != len(packed) || len(report.Entries) != 2 || report.Entries[0].Name != "func" ||
		report.Entries[0].SHA256 != hash([]byte("binary")) || report.Entries[0].Bytes != 6 {
		t.Fatalf("manifest mismatch: %s", manifest)
	}
	decoded, err := base64.StdEncoding.DecodeString(report.ArchiveSHA256Base64)
	if err != nil || hex.EncodeToString(decoded) != report.ArchiveSHA256 {
		t.Fatal("incorrect OCI archive digest")
	}
	slices.Reverse(entries)
	again, other, err := archive(entries)
	if err != nil || !bytes.Equal(packed, again) || !bytes.Equal(manifest, other) {
		t.Fatal("archive depends on input order")
	}
}

func TestArchiveRejectsInvalidInventory(t *testing.T) {
	for name, entries := range map[string][]entry{
		"empty":         nil,
		"no executable": {{"resources/notices/a/LICENSE", []byte("notice")}},
		"duplicate":     {{"func", []byte("one")}, {"func", []byte("two")}},
		"source":        {{"func", []byte("binary")}, {"internal/deployment/handler.go", []byte("source")}},
		"traversal":     {{"func", []byte("binary")}, {"resources/notices/../../secret", []byte("secret")}},
		"empty notice":  {{"func", []byte("binary")}, {"resources/notices/a/LICENSE", nil}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := archive(entries); err == nil {
				t.Fatal("accepted invalid archive")
			}
		})
	}
}

func TestArchiveRejectsOversizeActualZIP(t *testing.T) {
	data := make([]byte, 11_100_000)
	_, _ = rand.New(rand.NewSource(42)).Read(data)
	packed, manifest, err := archive([]entry{{"func", data}})
	if err == nil || packed != nil || manifest != nil {
		t.Fatal("oversize package accepted")
	}
}
