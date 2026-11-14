package reconciliation

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticIntentFieldBoundaries(t *testing.T) {
	for _, change := range []func(*DiagnosticIntent){
		func(i *DiagnosticIntent) { i.EnvironmentID = strings.Repeat("A", 64) }, func(i *DiagnosticIntent) { i.EnvironmentID = strings.Repeat("a", 63) },
		func(i *DiagnosticIntent) { i.InstallationID = strings.Repeat("a", 129) }, func(i *DiagnosticIntent) { i.RequestID = strings.Repeat("a", 129) },
		func(i *DiagnosticIntent) { i.RequestID = "test🙂" }, func(i *DiagnosticIntent) { i.ReleaseSHA256 = strings.Repeat("b", 65) }, func(i *DiagnosticIntent) { i.CatalogSHA256 = strings.Repeat("C", 64) },
		func(i *DiagnosticIntent) { i.SecretID = "ocid1.vaultsecret.oc2.iad.other" }, func(i *DiagnosticIntent) { i.SecretID = "ocid1.vaultsecret.oc1..other" }, func(i *DiagnosticIntent) { i.SecretID = "ocid1.vaultsecret.oc1.iad." + strings.Repeat("a", 4096) },
	} {
		i := diagnosticIntentFixture()
		change(&i)
		r, err := NewDiagnostic(i, 7, diagnosticClaimFixture())
		diagnosticRejected(t, r, err, "invalid_intent")
	}
	for _, secret := range []string{"ocid1.vaultsecret.oc1.us-ashburn-1.example", "ocid1.vaultsecret.oc1.iad..example", "ocid1.vaultsecret.oc1.iad.tenant.example"} {
		i := diagnosticIntentFixture()
		i.SecretID = secret
		i.PagerDutyRegion = "EU"
		if _, err := NewDiagnostic(i, math.MaxInt64, diagnosticClaimFixture()); err != nil {
			t.Fatal("valid reference boundary rejected", err)
		}
	}
}

func TestDiagnosticClaimClockAndTokenBoundaries(t *testing.T) {
	for _, change := range []func(*DiagnosticClaim){
		func(c *DiagnosticClaim) { c.OwnerToken = strings.Repeat("D", 64) }, func(c *DiagnosticClaim) { c.ClaimToken = strings.Repeat("e", 63) },
		func(c *DiagnosticClaim) { c.ExpiresAt = time.Time{} }, func(c *DiagnosticClaim) { c.Now = time.Date(0, 12, 31, 0, 0, 0, 0, time.UTC) },
		func(c *DiagnosticClaim) { c.ExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }, func(c *DiagnosticClaim) { c.ExpiresAt = c.Now.Add(-time.Second) },
		func(c *DiagnosticClaim) { c.ExpiresAt = c.Now.Add(time.Microsecond) },
	} {
		c := diagnosticClaimFixture()
		change(&c)
		r, err := NewDiagnostic(diagnosticIntentFixture(), 7, c)
		diagnosticRejected(t, r, err, "invalid_claim")
	}
	c := diagnosticClaimFixture()
	c.Now = c.Now.In(time.FixedZone("offset", 2*60*60)).Add(1234567 * time.Nanosecond)
	c.ExpiresAt = c.Now.Add(time.Millisecond)
	r, err := NewDiagnostic(diagnosticIntentFixture(), 7, c)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Snapshot()
	if s.StartedAt.Location() != time.UTC || s.StartedAt.Nanosecond() != 1000000 || s.ExpiresAt.Sub(s.StartedAt) != time.Millisecond {
		t.Fatal("clock normalization differs from canonical milliseconds")
	}
}

func TestDiagnosticMaximumFieldsFitEveryTerminalState(t *testing.T) {
	i := diagnosticIntentFixture()
	i.InstallationID = strings.Repeat("I", 128)
	i.RequestID = strings.Repeat("R", 128)
	prefix := "ocid1.vaultsecret.oc1.iad."
	i.SecretID = prefix + strings.Repeat("a", 4096-len(prefix))
	c := diagnosticClaimFixture()
	c.Now = time.Date(9999, 12, 31, 23, 49, 59, 999000000, time.UTC)
	c.ExpiresAt = c.Now.Add(600 * time.Second)
	r, err := NewDiagnostic(i, math.MaxInt64, c)
	if err != nil {
		t.Fatal(err)
	}
	records := []DiagnosticRecord{r}
	for _, status := range []int{0, 202, 429, 599} {
		next, err := r.Finish(diagnosticReference(c), status, c.ExpiresAt.Add(-time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, next)
	}
	expired, err := r.Expire(c.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	records = append(records, expired)
	for _, record := range records {
		body, err := record.Encode()
		if err != nil || len(body) >= diagnosticLimit {
			t.Fatal("maximum metadata exceeds fixed admission limit", len(body), err)
		}
		if len(record.Snapshot().PayloadWithoutRoutingKey) > 32<<10 {
			t.Fatal("diagnostic exceeds existing saved-envelope bound")
		}
		got, err := DecodeDiagnostic(body, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err != nil || got.Snapshot().State != record.Snapshot().State {
			t.Fatal("maximum valid state failed restart", err)
		}
	}
	i.SecretID += "a"
	bad, err := NewDiagnostic(i, 7, c)
	diagnosticRejected(t, bad, err, "invalid_intent")
}
