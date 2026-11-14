package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestStateBucketMustSupplyAUniqueOCID(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("storage.tf")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`prevent_destroy\s*=\s*true`).Match(source) {
		t.Fatal("installation bucket must remain protected")
	}
	match := regexp.MustCompile(`(?s)postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("missing unique bucket identity check")
	}
	for _, id := range []any{"ocid1.bucket.oc1.iad.example", "ocid1.bucket.oc1..example", "", nil, "n/example/b/name", "ocid1.bucket.oc2.iad.example"} {
		encoded, err := json.Marshal(id)
		if err != nil {
			t.Fatal(err)
		}
		expression := strings.ReplaceAll(string(match[1]), "self.bucket_id", string(encoded))
		output, err := terraform(t, t.TempDir(), strings.ReplaceAll(expression, "\n", " ")+"\n", "console", "-no-color")
		want := "false"
		if id == "ocid1.bucket.oc1.iad.example" || id == "ocid1.bucket.oc1..example" {
			want = "true"
		}
		if err != nil || strings.TrimSpace(string(output)) != want {
			t.Fatalf("bucket identity %v: %s (%v), want %s", id, output, err, want)
		}
	}
}

func TestInstallationIdentityRejection(t *testing.T) {
	for _, id := range []string{"", "per-apply-uuid", "b1-" + strings.Repeat("a", 63), "b1-" + strings.Repeat("G", 64)} {
		_, output, err := renderWithInstallation(t, settingsFixture(), id)
		if err == nil || !strings.Contains(output, "Installation identity") {
			t.Fatalf("invalid generated identity accepted: %s (%v)", output, err)
		}
	}
}
