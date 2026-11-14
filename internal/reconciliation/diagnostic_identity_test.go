package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticIndependentIdentityPayloadAndRecordGoldens(t *testing.T) {
	r := diagnosticFixture(t)
	s := r.Snapshot()
	if s.IntentDigest != "5153dfca4f59c0c82a40cdc7d6c5d2b8239fb2f5be037d10a5165d28a8759674" || s.DedupKey != "omr-test1:58cd72eebcad137ac4fcec3ddfd84eded03c8134eb91fde695f51eef972994ac" || s.ConfirmationCode != "OMR-6f041506e850f3fbedf3b7471c28337b" || s.PayloadDigest != "8d6f40d6daec8b6fb87eb8b7d7dc549a1c88677a9c32f9517194fb922676e8e6" {
		t.Fatal("independent intent/dedup/code/payload golden changed")
	}
	payload, err := os.ReadFile("testdata/diagnostic-payload.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, s.PayloadWithoutRoutingKey) {
		t.Fatal("fixed message changed")
	}
	want, err := os.ReadFile("testdata/diagnostic-sending.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Encode()
	if err != nil || !bytes.Equal(got, want) || fmt.Sprintf("%x", sha256.Sum256(got)) != "318513580224c72150acda3fec83ebe60de5d639013c0451e46f8e3e0cf79ef5" {
		t.Fatal("independent sending record golden changed", err)
	}
}

func TestDiagnosticIntentChangesRejectReuseButResolvedVersionAndTimeDoNot(t *testing.T) {
	r := diagnosticFixture(t)
	base := diagnosticIntentFixture()
	if !r.Matches(base) {
		t.Fatal("original intent rejected")
	}
	for _, change := range []func(*DiagnosticIntent){func(i *DiagnosticIntent) { i.EnvironmentID = strings.Repeat("f", 64) }, func(i *DiagnosticIntent) { i.InstallationID = "next" }, func(i *DiagnosticIntent) { i.RequestID = "next" }, func(i *DiagnosticIntent) { i.ReleaseSHA256 = strings.Repeat("f", 64) }, func(i *DiagnosticIntent) { i.CatalogSHA256 = strings.Repeat("f", 64) }, func(i *DiagnosticIntent) { i.PagerDutyRegion = "EU" }, func(i *DiagnosticIntent) { i.SecretID = "ocid1.vaultsecret.oc1.iad.other" }, func(i *DiagnosticIntent) { i.NonProduction = false }} {
		other := base
		change(&other)
		if r.Matches(other) {
			t.Fatal("changed intent reused request")
		}
		if other.NonProduction {
			next, err := NewDiagnostic(other, 7, diagnosticClaimFixture())
			if err != nil {
				t.Fatal(err)
			}
			if next.Snapshot().ConfirmationCode == r.Snapshot().ConfirmationCode {
				t.Fatal("new test retained old receipt correlation")
			}
		}
	}
	c := diagnosticClaimFixture()
	c.Now = c.Now.Add(time.Second)
	c.ExpiresAt = c.ExpiresAt.Add(time.Second)
	c.ClaimToken = strings.Repeat("f", 64)
	next, err := NewDiagnostic(base, 9, c)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Matches(base) || next.Snapshot().IntentDigest != r.Snapshot().IntentDigest || !bytes.Equal(next.Snapshot().PayloadWithoutRoutingKey, r.Snapshot().PayloadWithoutRoutingKey) {
		t.Fatal("resolved version/time/claim changed immutable intent/message")
	}
	if (DiagnosticRecord{}).Matches(base) {
		t.Fatal("zero record matched")
	}
}
