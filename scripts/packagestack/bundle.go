package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const bundleLimit = 11_000_000

// Explicit release inputs, not a recursive copy of the checkout.
var stackFiles = []string{
	".terraform.lock.hcl", "configuration.tf", "deployment_check.tf", "function.tf",
	"identity.tf", "identity_diagnostic.tf", "network.tf", "outputs.tf", "providers.tf", "release.json", "schema.yaml",
	"storage.tf", "variables_network.tf", "variables_operation.tf", "variables_placement.tf", "variables_routing.tf", "versions.tf",
	"modules/configuration/variables.tf", "modules/configuration/render.tf", "modules/configuration/outputs.tf",
	"operator.tf", "variables_operator.tf", "identity_operator.tf",
	"closure.tf", "variables_closure.tf", "modules/configuration/closure.tf",
	"activation.tf", "identity_live.tf",
	"logging.tf", "monitoring.tf",
	"automation.tf", "identity_automation.tf",
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func readInput(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("release input must be a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("release input exceeds size limit")
	}
	return data, nil
}

func bundle(root, functionDirectory string) ([]byte, error) {
	entries := map[string][]byte{}
	for _, name := range stackFiles {
		data, err := readInput(filepath.Join(root, "infra", filepath.FromSlash(name)), 1<<20)
		if err != nil {
			return nil, fmt.Errorf("stack input %s: %w", name, err)
		}
		entries[name] = data
	}
	for _, name := range []string{"function.zip", "function-manifest.json"} {
		limit := int64(bundleLimit)
		if name != "function.zip" {
			limit = 1 << 20
		}
		data, err := readInput(filepath.Join(functionDirectory, name), limit)
		if err != nil {
			return nil, fmt.Errorf("Function input %s: %w", name, err)
		}
		entries[name] = data
	}
	if err := verifyFunction(entries["function.zip"], entries["function-manifest.json"]); err != nil {
		return nil, err
	}
	return pack(entries)
}

func pack(entries map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		header.SetMode(0644)
		file, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(entries[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if output.Len() > bundleLimit {
		return nil, fmt.Errorf("complete RM ZIP exceeds %d bytes", bundleLimit)
	}
	return output.Bytes(), nil
}
