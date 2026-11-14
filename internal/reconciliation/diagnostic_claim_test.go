package reconciliation

import (
	"strings"
	"testing"
	"time"
)

func TestDiagnosticRejectsInvalidIntentVersionAndClaims(t *testing.T) {
	for _, change := range []func(*DiagnosticIntent){func(i *DiagnosticIntent) { i.EnvironmentID = "private" }, func(i *DiagnosticIntent) { i.InstallationID = "" }, func(i *DiagnosticIntent) { i.RequestID = "../private" }, func(i *DiagnosticIntent) { i.ReleaseSHA256 = "private" }, func(i *DiagnosticIntent) { i.CatalogSHA256 = "private" }, func(i *DiagnosticIntent) { i.PagerDutyRegion = "APAC" }, func(i *DiagnosticIntent) { i.SecretID = "private" }, func(i *DiagnosticIntent) { i.NonProduction = false }} {
		i := diagnosticIntentFixture()
		change(&i)
		r, err := NewDiagnostic(i, 7, diagnosticClaimFixture())
		diagnosticRejected(t, r, err, "invalid_intent")
	}
	for _, version := range []int64{0, -1} {
		r, err := NewDiagnostic(diagnosticIntentFixture(), version, diagnosticClaimFixture())
		diagnosticRejected(t, r, err, "invalid_version")
	}
	for _, change := range []func(*DiagnosticClaim){func(c *DiagnosticClaim) { c.OwnerToken = "private" }, func(c *DiagnosticClaim) { c.ClaimToken = "private" }, func(c *DiagnosticClaim) { c.Now = time.Time{} }, func(c *DiagnosticClaim) { c.ExpiresAt = c.Now }, func(c *DiagnosticClaim) { c.ExpiresAt = c.Now.Add(601 * time.Second) }} {
		c := diagnosticClaimFixture()
		change(&c)
		r, err := NewDiagnostic(diagnosticIntentFixture(), 7, c)
		diagnosticRejected(t, r, err, "invalid_claim")
	}
}

func TestDiagnosticSeparateOperationsUnderOneLeaseHaveDistinctClaims(t *testing.T) {
	a := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	b, err := NewDiagnostic(diagnosticIntentFixture(), 7, c)
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().OwnerToken != b.Snapshot().OwnerToken || a.Snapshot().ClaimToken == b.Snapshot().ClaimToken {
		t.Fatal("separate operations collapsed into lease identity")
	}
	wrong, err := a.Finish(diagnosticReference(c), 202, c.Now)
	diagnosticRejected(t, wrong, err, "stale_claim")
	wrong, err = b.Finish(diagnosticReference(diagnosticClaimFixture()), 202, c.Now)
	diagnosticRejected(t, wrong, err, "stale_claim")
}
