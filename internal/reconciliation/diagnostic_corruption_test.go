package reconciliation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func diagnosticCorruption(t *testing.T, r DiagnosticRecord, change func(*diagnosticData)) {
	t.Helper()
	d := *r.data
	change(&d)
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	i := diagnosticIntentFixture()
	got, err := DecodeDiagnostic(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err == nil || got.Snapshot().State != "" {
		t.Fatal("corrupted canonical record accepted")
	}
}

func TestDiagnosticCanonicalCodecValidatesIdentitiesAndClosedMessage(t *testing.T) {
	r := diagnosticFixture(t)
	for _, change := range []func(*diagnosticData){
		func(d *diagnosticData) { d.SchemaVersion = 0 }, func(d *diagnosticData) { d.Intent.NonProduction = false }, func(d *diagnosticData) { d.SecretVersion = 0 },
		func(d *diagnosticData) { d.OwnerToken = "wrong" }, func(d *diagnosticData) { d.ClaimToken = "wrong" }, func(d *diagnosticData) { d.IntentDigest = strings.Repeat("f", 64) },
		func(d *diagnosticData) { d.DedupKey = "omr1:other" }, func(d *diagnosticData) { d.PayloadDigest = strings.Repeat("f", 64) },
		func(d *diagnosticData) {
			d.Payload = strings.Replace(d.Payload, "trigger", "resolve", 1)
			d.PayloadDigest = diagnosticDigest(d.Payload)
		},
		func(d *diagnosticData) {
			d.Payload = strings.Replace(d.Payload, `"payload":{`, `"routing_key":"synthetic-value","payload":{`, 1)
			d.PayloadDigest = diagnosticDigest(d.Payload)
		},
		func(d *diagnosticData) {
			d.Payload = strings.Replace(d.Payload, "OMR-", "OTHER-", 1)
			d.PayloadDigest = diagnosticDigest(d.Payload)
		},
	} {
		diagnosticCorruption(t, r, change)
	}
}

func TestDiagnosticCanonicalCodecValidatesStateAndTimeMatrix(t *testing.T) {
	r := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	accepted, _ := r.Finish(diagnosticReference(c), 202, c.Now)
	unknown, _ := r.Finish(diagnosticReference(c), 0, c.Now)
	response, _ := r.Finish(diagnosticReference(c), 503, c.Now)
	expired, _ := r.Expire(c.ExpiresAt)
	for _, tc := range []struct {
		record DiagnosticRecord
		change func(*diagnosticData)
	}{
		{r, func(d *diagnosticData) { d.StartedAt = "bad" }}, {r, func(d *diagnosticData) { d.ExpiresAt = "bad" }}, {r, func(d *diagnosticData) { d.UpdatedAt = "bad" }},
		{r, func(d *diagnosticData) { d.ExpiresAt = d.StartedAt }}, {r, func(d *diagnosticData) { d.ExpiresAt = c.ExpiresAt.Add(time.Millisecond).Format(leaseTimeFormat) }}, {r, func(d *diagnosticData) { d.UpdatedAt = c.Now.Add(-time.Millisecond).Format(leaseTimeFormat) }},
		{r, func(d *diagnosticData) { d.State = "RETRY_WAIT" }}, {r, func(d *diagnosticData) { d.HTTPStatus = 202 }}, {r, func(d *diagnosticData) { d.Reason = "response" }}, {r, func(d *diagnosticData) { d.Ambiguous = true }},
		{r, func(d *diagnosticData) { v := d.StartedAt; d.AcceptedAt = &v }}, {r, func(d *diagnosticData) { d.UpdatedAt = c.Now.Add(time.Millisecond).Format(leaseTimeFormat) }},
		{accepted, func(d *diagnosticData) { d.AcceptedAt = nil }}, {accepted, func(d *diagnosticData) { d.HTTPStatus = 200 }}, {accepted, func(d *diagnosticData) { d.Ambiguous = true }}, {accepted, func(d *diagnosticData) { d.Reason = "expired" }},
		{accepted, func(d *diagnosticData) { v := d.ExpiresAt; d.AcceptedAt = &v }}, {accepted, func(d *diagnosticData) { d.UpdatedAt = d.ExpiresAt; v := d.UpdatedAt; d.AcceptedAt = &v }},
		{unknown, func(d *diagnosticData) { d.Reason = "private" }}, {unknown, func(d *diagnosticData) { d.HTTPStatus = 400 }}, {unknown, func(d *diagnosticData) { d.Ambiguous = false }}, {unknown, func(d *diagnosticData) { d.UpdatedAt = d.ExpiresAt }},
		{response, func(d *diagnosticData) { d.HTTPStatus = 202 }}, {response, func(d *diagnosticData) { d.HTTPStatus = 99 }}, {response, func(d *diagnosticData) { d.HTTPStatus = 600 }}, {response, func(d *diagnosticData) { d.Ambiguous = false }},
		{response, func(d *diagnosticData) { d.UpdatedAt = d.ExpiresAt }}, {response, func(d *diagnosticData) { v := d.UpdatedAt; d.AcceptedAt = &v }},
		{expired, func(d *diagnosticData) { d.UpdatedAt = d.StartedAt }}, {expired, func(d *diagnosticData) { d.Ambiguous = false }}, {expired, func(d *diagnosticData) { d.HTTPStatus = 202 }},
	} {
		diagnosticCorruption(t, tc.record, tc.change)
	}
}
