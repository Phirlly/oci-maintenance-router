package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealTestRunner(t *testing.T) {
	for _, tc := range []struct {
		name, extraPath, extraSource, result string
		status                               int
	}{
		{
			name:   "covered package passes",
			result: "PASS: statement coverage 100.00% (1/1); required >=97%",
		},
		{
			name:        "package without tests counts against coverage",
			extraPath:   "untested/sample.go",
			extraSource: "package untested\nfunc Value() int { return 2 }\n",
			result:      "FAIL: statement coverage 50.00% (1/2); required >=97%",
			status:      1,
		},
		{
			name:        "failing test cannot pass",
			extraPath:   "failure_test.go",
			extraSource: "package sample\nimport \"testing\"\nfunc TestFailure(t *testing.T) { t.Fatal(\"fixture failure\") }\n",
			result:      "coverage: tests failed:",
			status:      1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository(t)
			files := map[string]string{
				"sample.go":      "package sample\nfunc Value() int { return 1 }\n",
				"sample_test.go": "package sample\nimport \"testing\"\nfunc TestValue(t *testing.T) { if Value() != 1 { t.Fatal(\"wrong value\") } }\n",
			}
			if tc.extraPath != "" {
				files[tc.extraPath] = tc.extraSource
			}
			for path, source := range files {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			if status := run(nil, &output, runTests); status != tc.status {
				t.Fatalf("status=%d want=%d output=%s", status, tc.status, output.String())
			}
			if !strings.Contains(output.String(), tc.result) {
				t.Fatalf("missing result %q: %s", tc.result, output.String())
			}
			if tc.status != 0 && strings.Contains(output.String(), "PASS:") {
				t.Fatalf("failed gate reported success: %s", output.String())
			}
		})
	}
}
