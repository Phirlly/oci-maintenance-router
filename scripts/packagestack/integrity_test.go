package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFunctionInventoryRejectsMismatch(t *testing.T) {
	for _, kind := range []string{"purpose", "duplicate", "name", "size", "checksum", "binary", "zip"} {
		t.Run(kind, func(t *testing.T) {
			_, dir := inputFixture(t)
			archive, err := os.ReadFile(filepath.Join(dir, "function.zip"))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "function-manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var report functionManifest
			if json.Unmarshal(data, &report) != nil {
				t.Fatal("fixture")
			}
			switch kind {
			case "purpose":
				report.Purpose = "deployment-check-only"
			case "duplicate":
				report.Entries[1].Name = report.Entries[0].Name
			case "name":
				report.Entries[0].Name = "unexpected"
			case "size":
				report.Entries[0].Bytes++
			case "checksum":
				report.Entries[1].SHA256 = "bad"
			case "binary":
				report.BinarySHA256 = "bad"
			case "zip":
				archive = []byte("not a ZIP")
				hash := sha256.Sum256(archive)
				report.ArchiveBytes = len(archive)
				report.ArchiveSHA256 = digest(archive)
				report.ArchiveSHA256Base64 = base64.StdEncoding.EncodeToString(hash[:])
			}
			data, err = json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyFunction(archive, data, nil); err == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
}

func TestCompleteBundleLimitCountsCompressedZIP(t *testing.T) {
	// Deterministic incompressible fixture checks the complete outer ZIP budget.
	data := make([]byte, bundleLimit)
	r := rand.NewChaCha8([32]byte{7})
	for i := range data {
		data[i] = byte(r.Uint64())
	}
	if _, err := pack(map[string][]byte{"function.zip": data}); err == nil {
		t.Fatal("oversized RM ZIP accepted")
	}
}

func TestBundleRequiresEveryAllowlistedFile(t *testing.T) {
	for _, name := range stackFiles {
		t.Run(name, func(t *testing.T) {
			root, dir := localStackFixture(t)
			if err := os.Remove(filepath.Join(root, "infra", filepath.FromSlash(name))); err != nil {
				t.Fatal(err)
			}
			if _, err := bundle(root, dir); err == nil || !strings.Contains(err.Error(), "stack input "+name+":") {
				t.Fatalf("missing infrastructure was not identified: %v", err)
			}
		})
	}
}

func TestFunctionManifestRejectsAffectedCompilerPatch(t *testing.T) {
	_, dir := inputFixture(t)
	archive, err := os.ReadFile(filepath.Join(dir, "function.zip"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "function-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report functionManifest
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	report.GoVersion = "go1.27.3"
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if verifyFunction(archive, data, nil) == nil {
		t.Fatal("accepted an archive from the compiler patch affected by GO-2026-6859")
	}
}
