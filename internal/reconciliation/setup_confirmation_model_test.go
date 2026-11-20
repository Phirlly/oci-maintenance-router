package reconciliation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func setupProofs(t *testing.T) (*liveSelectorFixture, DiagnosticReceipt, OperatorReceipt) {
	t.Helper()
	f := liveSelectorFixtureFor(t)
	d, err := f.selector.diagnostic.Verify(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.selector.operator.Verify(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	return f, d, o
}

func TestSetupConfirmationBindsGenuineAcceptedPair(t *testing.T) {
	f, d, o := setupProofs(t)
	now := f.lease.manager.now()
	r, err := newSetupConfirmation(d, o, now)
	if err != nil || r.ID() == "" {
		t.Fatalf("genuine paired confirmation rejected: %v", err)
	}
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	db, _ := d.Record().Encode()
	ob, _ := o.Record().Encode()
	if fields["diagnosticSHA256"] != diagnosticDigest(string(db)) || fields["operatorSHA256"] != diagnosticDigest(string(ob)) {
		t.Fatal("confirmation did not bind complete accepted records")
	}
	for _, private := range []string{d.Record().Snapshot().ConfirmationCode, o.Record().Snapshot().ConfirmationCode, "synthetic-key", "@", "secretId", "ownerToken", "payload"} {
		if bytes.Contains(body, []byte(private)) {
			t.Fatalf("confirmation contains %q", private)
		}
	}
	// Configuration and time are provenance; route edits cannot change pair identity.
	d.configHash[0] ^= 1
	o.configHash = d.configHash
	later, err := newSetupConfirmation(d, o, now.Add(time.Second))
	if err != nil || later.ID() != r.ID() {
		t.Fatal("same pair lost stable identity", err)
	}
}

func TestSetupConfirmationRejectsMissingMixedOrPrematureProof(t *testing.T) {
	f, d, o := setupProofs(t)
	for _, kind := range []string{"diagnostic", "operator", "mixed", "zero-hash", "zero-time", "before-accepted"} {
		t.Run(kind, func(t *testing.T) {
			diag, operator, at := d, o, f.lease.manager.now()
			switch kind {
			case "diagnostic":
				diag = DiagnosticReceipt{}
			case "operator":
				operator = OperatorReceipt{}
			case "mixed":
				operator.configHash[0] ^= 1
			case "zero-hash":
				diag.configHash, operator.configHash = [32]byte{}, [32]byte{}
			case "zero-time":
				at = time.Time{}
			case "before-accepted":
				at = d.Record().Snapshot().AcceptedAt.Add(-time.Second)
			}
			r, err := newSetupConfirmation(diag, operator, at)
			if err == nil || r.ID() != "" {
				t.Fatal("invalid pair returned confirmation")
			}
		})
	}
	_, other, _ := setupProofs(t)
	// Deliberately corrupted same-package evidence cannot join another installation.
	data := *other.record.data
	data.Intent.InstallationID = strings.Repeat("z", 12)
	other.record.data = &data
	if _, err := newSetupConfirmation(other, o, f.lease.manager.now()); err == nil {
		t.Fatal("foreign installation accepted")
	}
}
