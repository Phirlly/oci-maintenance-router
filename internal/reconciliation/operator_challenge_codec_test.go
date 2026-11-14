package reconciliation

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestOperatorChallengeCanonicalGoldenRestarts(t *testing.T) {
	body, err := os.ReadFile("testdata/operator-publishing.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := DecodeOperatorChallenge(body, strings.Repeat("a", 64), "installation-1", "operator-test-1")
	if err != nil || r.Snapshot().State != OperatorPublishing {
		t.Fatal("canonical challenge rejected", err)
	}
	encoded, err := r.Encode()
	if err != nil || !bytes.Equal(body, encoded) {
		t.Fatal("restart changed bytes", err)
	}
}
