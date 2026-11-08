package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func run(args []string, output io.Writer, execute func(string, io.Writer) error) int {
	if len(args) != 0 {
		fmt.Fprintln(output, "usage: go run ./scripts/coverage (from repository root; no arguments)")
		return 2
	}
	if !repositoryRoot() {
		fmt.Fprintln(output, "coverage: run from the oci-maintenance-router repository root")
		return 1
	}
	directory, err := os.MkdirTemp("", "oci-router-coverage-")
	if err != nil {
		fmt.Fprintln(output, "coverage: cannot create temporary report:", err)
		return 1
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "coverage.out")
	if err := execute(path, output); err != nil {
		fmt.Fprintln(output, "coverage: tests failed:", err)
		return 1
	}
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(output, "coverage: cannot read report:", err)
		return 1
	}
	defer file.Close()
	result, err := readCoverage(file)
	if err != nil {
		fmt.Fprintln(output, "coverage: invalid report:", err)
		return 1
	}
	status, code := "PASS", 0
	if !result.passes() {
		status, code = "FAIL", 1
	}
	fmt.Fprintf(output, "%s: statement coverage %.2f%% (%d/%d); required >=%d%%\n",
		status, 100*float64(result.covered)/float64(result.total), result.covered, result.total, minimumPercent)
	return code
}

func repositoryRoot() bool {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"") == "oci-maintenance-router"
		}
	}
	return false
}
