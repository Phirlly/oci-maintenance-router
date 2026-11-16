package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageExcludesImportedDependencyInsideCheckout(t *testing.T) {
	repository(t)
	files := map[string]string{
		"go.mod": "module oci-maintenance-router\n\ngo 1.27.4\n\n" +
			"require example.invalid/dependency v0.0.0\n" +
			"replace example.invalid/dependency => ./.local/dependency\n",
		".local/dependency/go.mod": "module example.invalid/dependency\n\ngo 1.27.4\n",
		".local/dependency/value.go": "package dependency\n" +
			"func Value() int { return 1 }\n" +
			"func Unused() int { return 2 }\n",
		"sample.go": "package sample\nimport \"example.invalid/dependency\"\n" +
			"func Value() int { return dependency.Value() }\n",
		"sample_test.go": "package sample\nimport \"testing\"\n" +
			"func TestValue(t *testing.T) { if Value() != 1 { t.Fatal(\"wrong value\") } }\n",
	}
	for name, source := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	status := run(nil, &output, runTests)
	if status != 0 || !strings.Contains(output.String(), "PASS: statement coverage 100.00% (1/1); required >=97%") {
		t.Fatalf("coverage must count only owned statements: status=%d output=%s", status, &output)
	}
}
