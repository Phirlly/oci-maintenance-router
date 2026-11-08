package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

type functionManifest struct {
	SchemaVersion                                     int
	Purpose, Architecture, OperatingSystem, GoVersion string
	ArchiveSHA256                                     string
	ArchiveSHA256Base64                               string
	ArchiveBytes                                      int
	BinarySHA256                                      string
	Entries                                           []struct {
		Name, SHA256 string
		Bytes        int
	}
}

func verifyFunction(archive, manifest []byte) error {
	var report functionManifest
	hash := sha256.Sum256(archive)
	if json.Unmarshal(manifest, &report) != nil || report.SchemaVersion != 1 || report.Purpose != "deployment-check-only" ||
		report.Architecture != "X86" || report.OperatingSystem != "linux" || report.GoVersion != "go1.27.3" ||
		report.ArchiveBytes != len(archive) || report.ArchiveSHA256 != digest(archive) || report.ArchiveSHA256Base64 != base64.StdEncoding.EncodeToString(hash[:]) {
		return fmt.Errorf("Function manifest identity or archive checksum mismatch")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return err
	}
	if len(reader.File) != len(report.Entries) || len(reader.File) < 2 {
		return fmt.Errorf("Function inventory mismatch")
	}
	expected := map[string]int{}
	for i, e := range report.Entries {
		if _, exists := expected[e.Name]; exists {
			return fmt.Errorf("duplicate manifest entry")
		}
		expected[e.Name] = i
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range reader.File {
		index, ok := expected[f.Name]
		if !ok || seen[f.Name] || path.Clean(f.Name) != f.Name || strings.Contains(f.Name, "\\") ||
			!f.Mode().IsRegular() || (f.Name != "func" && !strings.HasPrefix(f.Name, "resources/notices/")) {
			return fmt.Errorf("unexpected Function archive entry")
		}
		seen[f.Name] = true
		e := report.Entries[index]
		if e.Bytes <= 0 || f.UncompressedSize64 > 64<<20 || int64(e.Bytes) != int64(f.UncompressedSize64) {
			return fmt.Errorf("Function entry size mismatch")
		}
		total += int64(e.Bytes)
		if total > 64<<20 {
			return fmt.Errorf("Function expanded inventory exceeds packaging budget")
		}
		source, err := f.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(source, int64(e.Bytes)+1))
		closeErr := source.Close()
		if readErr != nil || closeErr != nil || len(data) != e.Bytes || digest(data) != e.SHA256 {
			return fmt.Errorf("Function entry checksum mismatch")
		}
		if f.Name == "func" && (f.Mode().Perm() != 0755 || digest(data) != report.BinarySHA256) {
			return fmt.Errorf("Function executable mismatch")
		}
	}
	if !seen["func"] {
		return fmt.Errorf("Function executable missing")
	}
	return nil
}
