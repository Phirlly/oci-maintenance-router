package infra_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestFunctionArchiveChecksumEncodings(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("function.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("cannot locate actual Function checksum postcondition")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "function.zip")
	data := []byte("synthetic archive\x00\xff")
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	wrong := sha256.Sum256(append(append([]byte(nil), data...), 1))
	hexDigest := hex.EncodeToString(digest[:])
	base64Digest := base64.StdEncoding.EncodeToString(digest[:])
	for _, tc := range []struct {
		name     string
		reported any
		valid    bool
	}{
		{"hex", hexDigest, true},
		{"base64", base64Digest, true},
		{"wrong_hex", hex.EncodeToString(wrong[:]), false},
		{"wrong_base64", base64.StdEncoding.EncodeToString(wrong[:]), false},
		{"empty", "", false},
		{"null", nil, false},
		{"uppercase_hex", strings.ToUpper(hexDigest), false},
		{"leading_space", " " + hexDigest, false},
		{"trailing_newline", base64Digest + "\n", false},
		{"truncated", hexDigest[:63], false},
		{"prefixed", "sha256:" + hexDigest, false},
		{"encoded_hex_text", base64.StdEncoding.EncodeToString([]byte(hexDigest)), false},
		{"malformed", "not a checksum", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reported, err := json.Marshal(tc.reported)
			if err != nil {
				t.Fatal(err)
			}
			path, err := json.Marshal(archive)
			if err != nil {
				t.Fatal(err)
			}
			expression := strings.NewReplacer(
				"self.source_details[0].source_code_sha256", string(reported),
				"local.archive_path", string(path), "\n", " ",
			).Replace(string(match[1]))
			output, err := terraform(t, dir, expression+"\n", "console", "-no-color")
			if err != nil || strings.TrimSpace(string(output)) != strconv.FormatBool(tc.valid) {
				t.Fatalf("checksum predicate got %s (%v), want %t", output, err, tc.valid)
			}
		})
	}
}
