package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func run(args []string, out io.Writer) int {
	if len(args) != 2 || !filepath.IsAbs(args[0]) || !filepath.IsAbs(args[1]) {
		fmt.Fprintln(out, "usage: go run ./scripts/packagestack /absolute/function-directory /absolute/new-stack.zip (from repository root)")
		return 2
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(out, "package:", err)
		return 1
	}
	data, err := readInput(filepath.Join(root, "go.mod"), 1<<20)
	if err != nil || !strings.HasPrefix(string(data), "module oci-maintenance-router\n") {
		fmt.Fprintln(out, "package: run from the oci-maintenance-router repository root")
		return 1
	}
	output, err := outputPath(root, args[1])
	if err != nil {
		fmt.Fprintln(out, "package:", err)
		return 1
	}
	archive, err := bundle(root, args[0])
	if err != nil {
		fmt.Fprintln(out, "package:", err)
		return 1
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(out, "package: output must be new:", err)
		return 1
	}
	_, writeErr := file.Write(archive)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		if err := os.Remove(output); err != nil {
			fmt.Fprintln(out, "package: cannot remove partial output:", err)
		}
		fmt.Fprintln(out, "package: cannot write bundle")
		return 1
	}
	fmt.Fprintf(out, "Built RM preview/test bundle: %d bytes, SHA256 %s\n", len(archive), digest(archive))
	return 0
}

func outputPath(root, requested string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	local, _ := os.Lstat(filepath.Join(root, ".local"))
	parent, err := filepath.EvalSymlinks(filepath.Dir(requested))
	if err != nil {
		return "", err
	}
	for current := parent; ; current = filepath.Dir(current) {
		other, err := os.Stat(current)
		if err != nil {
			return "", err
		}
		if local != nil && local.IsDir() && os.SameFile(local, other) {
			break
		}
		if os.SameFile(info, other) {
			return "", fmt.Errorf("output must be outside the checkout or beneath its real .local directory")
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return filepath.Join(parent, filepath.Base(requested)), nil
}
