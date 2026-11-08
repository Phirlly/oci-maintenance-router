package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestExactCoverageThreshold(t *testing.T) {
	for _, tc := range []struct {
		name           string
		covered, total uint64
		pass           bool
	}{
		{"exact floor", 97, 100, true},
		{"above floor", 98, 100, true},
		{"below floor", 96, 100, false},
		{"rounded display trap", 9696, 10000, false},
		{"very close below", 969999, 1000000, false},
		{"zero coverage", 0, 100, false},
		{"all covered", 100, 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := fmt.Sprintf("mode: set\nexample/a.go:1.1,2.1 %d 1\nexample/b.go:1.1,2.1 %d 0\n", tc.covered, tc.total-tc.covered)
			got, err := readCoverage(strings.NewReader(profile))
			if err != nil {
				t.Fatal(err)
			}
			if got.covered != tc.covered || got.total != tc.total || got.passes() != tc.pass {
				t.Fatalf("coverage=%+v passes=%v; want %d/%d passes=%v", got, got.passes(), tc.covered, tc.total, tc.pass)
			}
		})
	}
}

func TestStatementWeightsAndDuplicateBlocks(t *testing.T) {
	for _, mode := range []string{"set", "count", "atomic"} {
		profile := "mode: " + mode + "\nexample/a.go:1.1,2.1 97 0\nexample/a.go:1.1,2.1 97 1\n" +
			"example/b.go:1.1,2.1 3 0\nexample/a.go:1.1,2.1 97 1000\n"
		got, err := readCoverage(strings.NewReader(profile))
		if err != nil || got != (coverage{97, 100}) || !got.passes() {
			t.Fatalf("%s: coverage=%+v err=%v", mode, got, err)
		}
	}
}

func TestProfileRejectsUnusableData(t *testing.T) {
	for name, profile := range map[string]string{
		"empty":                 "",
		"no statements":         "mode: set\n",
		"zero statements":       "mode: set\na.go:1.1,2.1 0 1\n",
		"bad mode":              "mode: unknown\na.go:1.1,2.1 1 1\n",
		"missing mode":          "a.go:1.1,2.1 1 1\n",
		"bad record":            "mode: set\nnot a block\n",
		"missing count":         "mode: set\na.go:1.1,2.1 1\n",
		"negative statements":   "mode: set\na.go:1.1,2.1 -1 1\n",
		"negative count":        "mode: set\na.go:1.1,2.1 1 -1\n",
		"conflicting duplicate": "mode: set\na.go:1.1,2.1 97 1\na.go:1.1,2.1 3 0\n",
		"statement overflow":    "mode: set\na.go:1.1,2.1 18446744073709551616 1\n",
		"count overflow":        "mode: set\na.go:1.1,2.1 1 18446744073709551616\n",
		"position overflow":     "mode: set\na.go:18446744073709551616.1,2.1 1 1\n",
		"sum overflow":          "mode: set\na.go:1.1,2.1 184467440737095516 1\nb.go:1.1,2.1 1 0\n",
		"too many statements":   "mode: set\na.go:1.1,2.1 184467440737095517 1\n",
		"truncated line":        "mode: set\n" + strings.Repeat("x", 70000),
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := readCoverage(strings.NewReader(profile)); err == nil {
				t.Fatalf("accepted unusable profile: %+v", got)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestProfileReadFailure(t *testing.T) {
	input := io.MultiReader(strings.NewReader("mode: set\na.go:1.1,2.1 1 1\n"), failingReader{})
	if _, err := readCoverage(input); err == nil {
		t.Fatal("accepted partial profile after read failure")
	}
}
