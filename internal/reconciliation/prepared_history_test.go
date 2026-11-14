package reconciliation

import (
	"bytes"
	"strings"
	"testing"
)

func TestPreparedHistoryEmptyCanonicalRecordAndRestart(t *testing.T) {
	env := strings.Repeat("a", 64)
	h, err := NewPreparedHistory(env, "installation-1")
	if err != nil {
		t.Fatal("valid initial history rejected", err)
	}
	want := []byte(`{"schemaVersion":1,"environmentId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","installationId":"installation-1","preparedBaseline":null}`)
	body, err := h.Encode()
	if err != nil || !bytes.Equal(body, want) || h.EnvironmentID() != env || h.InstallationID() != "installation-1" || h.BaselineID() != "" || h.BaselineDigest() != "" {
		t.Fatal("initial record identity or canonical bytes changed", err)
	}
	got, err := DecodePreparedHistory(body, env, "installation-1")
	clear(body)
	again, encodeErr := got.Encode()
	if err != nil || encodeErr != nil || !bytes.Equal(again, want) {
		t.Fatal("restart changed or retained mutable bytes", err, encodeErr)
	}
}

func TestPreparedHistoryAdvancesExactlyOneBaselineLayer(t *testing.T) {
	b := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	h, err := NewPreparedHistory(b.EnvironmentID(), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := h.Advance(Baseline{}, b)
	if err != nil || first.BaselineID() != b.ID() || len(first.BaselineDigest()) != 64 || h.BaselineID() != "" {
		t.Fatal("initial baseline was not retained independently", err)
	}
	next, err := b.Extend("expanded", baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
	if err != nil {
		t.Fatal(err)
	}
	second, err := first.Advance(b, next)
	if err != nil || second.BaselineID() != "expanded" || second.BaselineDigest() == first.BaselineDigest() || first.BaselineID() != "initial" {
		t.Fatal("cumulative extension failed", err)
	}
}
