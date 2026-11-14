package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func run(args []string, out io.Writer, build func(string) ([]entry, error)) int {
	if len(args) != 1 || !filepath.IsAbs(args[0]) {
		fmt.Fprintln(out, "usage: go run ./scripts/packagefunction /absolute/new-output-directory (from repository root)")
		return 2
	}
	directory, err := outputDirectory(args[0])
	if err != nil {
		fmt.Fprintln(out, "package:", err)
		return 1
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		fmt.Fprintln(out, "package: output must be new:", err)
		return 1
	}
	complete := false
	defer func() {
		if !complete {
			if err := os.RemoveAll(directory); err != nil {
				fmt.Fprintln(out, "package: cannot remove partial output:", err)
			}
		}
	}()
	entries, err := build(directory)
	if err != nil {
		fmt.Fprintln(out, "package: build failed:", err)
		return 1
	}
	packed, manifest, err := archive(entries)
	if err != nil {
		fmt.Fprintln(out, "package:", err)
		return 1
	}
	sums := digest(entries[0].data) + "  func\n"
	for _, artifact := range []entry{{"function.zip", packed}, {"function-manifest.json", manifest}} {
		if err := os.WriteFile(filepath.Join(directory, artifact.name), artifact.data, 0644); err != nil {
			fmt.Fprintln(out, "package: write failed:", err)
			return 1
		}
		sums += digest(artifact.data) + "  " + artifact.name + "\n"
	}
	if err := os.WriteFile(filepath.Join(directory, "SHA256SUMS"), []byte(sums), 0644); err != nil {
		fmt.Fprintln(out, "package: checksums write failed:", err)
		return 1
	}
	complete = true
	fmt.Fprintf(out, "Built Function archive: %d bytes, SHA256 %s\n", len(packed), digest(packed))
	return 0
}

func outputDirectory(requested string) (string, error) {
	module, err := os.ReadFile("go.mod")
	if err != nil || !strings.HasPrefix(string(module), "module oci-maintenance-router\n") {
		return "", fmt.Errorf("run from the oci-maintenance-router repository root")
	}
	root, err := os.Stat(".")
	if err != nil {
		return "", err
	}
	// Resolve symlinks, then compare directory identities to catch case aliases too.
	parent, err := filepath.EvalSymlinks(filepath.Dir(requested))
	if err != nil {
		return "", err
	}
	for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Stat(ancestor)
		if err != nil {
			return "", err
		}
		if os.SameFile(root, info) {
			return "", fmt.Errorf("output must be outside the repository")
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	return filepath.Join(parent, filepath.Base(requested)), nil
}
