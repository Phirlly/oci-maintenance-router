package reconciliation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func diagnosticIntentFixture() DiagnosticIntent {
	return DiagnosticIntent{EnvironmentID: strings.Repeat("a", 64), InstallationID: "installation-1", RequestID: "test-1", ReleaseSHA256: strings.Repeat("b", 64), CatalogSHA256: strings.Repeat("c", 64), PagerDutyRegion: "US", SecretID: "ocid1.vaultsecret.oc1.iad.diagnostic", NonProduction: true}
}
func diagnosticClaimFixture() DiagnosticClaim {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	return DiagnosticClaim{OwnerToken: strings.Repeat("d", 64), ClaimToken: strings.Repeat("e", 64), Now: now, ExpiresAt: now.Add(600 * time.Second)}
}
func diagnosticReference(c DiagnosticClaim) DiagnosticClaimRef {
	return DiagnosticClaimRef{c.OwnerToken, c.ClaimToken}
}
func diagnosticFixture(t *testing.T) DiagnosticRecord {
	t.Helper()
	r, err := NewDiagnostic(diagnosticIntentFixture(), 7, diagnosticClaimFixture())
	if err != nil || r.Snapshot().State != DiagnosticSending {
		t.Fatalf("valid diagnostic claim rejected: %v", err)
	}
	return r
}
func diagnosticRejected(t *testing.T, r DiagnosticRecord, err error, code string) {
	t.Helper()
	var detail *DiagnosticError
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v; want %s", err, code)
	}
	if r.Snapshot().State != "" || r.Snapshot().PayloadWithoutRoutingKey != nil {
		t.Fatal("rejected record returned partial state")
	}
}
