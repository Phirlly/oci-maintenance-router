package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

const archiveLimit = 11_000_000

type entry struct {
	name string
	data []byte
}
type inventoryEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}
type manifest struct {
	SchemaVersion       int              `json:"schemaVersion"`
	Purpose             string           `json:"purpose"`
	GoVersion           string           `json:"goVersion"`
	Architecture        string           `json:"architecture"`
	OperatingSystem     string           `json:"operatingSystem"`
	ArchiveSHA256       string           `json:"archiveSha256"`
	ArchiveSHA256Base64 string           `json:"archiveSha256Base64"`
	BinarySHA256        string           `json:"binarySha256"`
	ArchiveBytes        int              `json:"archiveBytes"`
	Dependencies        []dependency     `json:"dependencies"`
	Entries             []inventoryEntry `json:"entries"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func archive(input []entry) ([]byte, []byte, error) {
	entries := slices.Clone(input)
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.name, b.name) })
	if len(entries) == 0 || entries[0].name != "func" {
		return nil, nil, fmt.Errorf("archive needs root func")
	}
	for i, e := range entries {
		if len(e.data) == 0 || (i > 0 && e.name == entries[i-1].name) ||
			path.Clean(e.name) != e.name || strings.Contains(e.name, "\\") ||
			(e.name != "func" && !strings.HasPrefix(e.name, "resources/notices/")) {
			return nil, nil, fmt.Errorf("invalid archive inventory")
		}
	}
	report := manifest{SchemaVersion: 1, Purpose: "maintenance-router-runtime", GoVersion: compilerVersion,
		Architecture: "X86", OperatingSystem: "linux", BinarySHA256: digest(entries[0].data)}
	for _, module := range reviewedDependencies() {
		report.Dependencies = append(report.Dependencies, module.dependency)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		header.SetMode(0644)
		if e.name == "func" {
			header.SetMode(0755)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			return nil, nil, err
		}
		if _, err := file.Write(e.data); err != nil {
			return nil, nil, err
		}
		report.Entries = append(report.Entries, inventoryEntry{e.name, digest(e.data), len(e.data)})
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	if output.Len() > archiveLimit {
		return nil, nil, fmt.Errorf("Function ZIP exceeds %d bytes; complete RM bundle must also fit", archiveLimit)
	}
	report.ArchiveSHA256 = digest(output.Bytes())
	sum := sha256.Sum256(output.Bytes())
	report.ArchiveSHA256Base64 = base64.StdEncoding.EncodeToString(sum[:])
	report.ArchiveBytes = output.Len()
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return output.Bytes(), append(encoded, '\n'), nil
}
