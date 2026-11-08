package delivery

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func codecFixture(t *testing.T, reference string) (Record, Preparation) {
	t.Helper()
	data, err := os.ReadFile("testdata/config.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(data, configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}, configuration.LedgerLocation{Namespace: "example_namespace", Bucket: "Router.Ledger"})
	if err != nil {
		t.Fatal(err)
	}
	id, comp, platform := "ocid1.mysqldbsystem.oc1.iad.example", "ocid1.compartment.oc1..example", "IAAS"
	n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE", Platform: &platform, Services: []string{"Oracle HeatWave MySQL"}, Reference: &reference, Resources: []notices.Resource{{ResourceID: id}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Build(n, false, map[string]routing.Lookup{id: {State: routing.Found, Metadata: routing.Metadata{ID: &id, CompartmentID: &comp, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "payments"}}}}}, []string{id})
	if err != nil || len(result.Drafts) != 1 {
		t.Fatalf("fixture: %v", err)
	}
	p := Preparation{Draft: result.Drafts[0], SecretVersion: 7}
	r, err := NewRecord(p)
	if err != nil {
		t.Fatal(err)
	}
	return r, p
}

func decodeData(d recordData) (Record, error) {
	data, _ := json.Marshal(d)
	return DecodeRecord(data, d.EnvironmentID, d.NoticeID)
}

func rejectedData(t *testing.T, d recordData) {
	t.Helper()
	if r, err := decodeData(d); err == nil || r.data != nil {
		t.Fatal("corrupted canonical state accepted")
	}
}

func TestDecodeValidatesStateMatrix(t *testing.T) {
	p, _ := codecFixture(t, "sample")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := p.data.Batches[0].Message.BatchID
	s, ref, err := p.Begin(id, AttemptContext{OwnerToken: "owner", RoutingGeneration: p.data.Batches[0].Message.RoutingGeneration, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.Finish(ref, 202, now, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.Finish(ref, 503, now, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	held, err := p.Hold(id, ReasonOperator, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		base   Record
		change func(*savedBatch)
	}{
		{"prepared owner", p, func(b *savedBatch) { b.OwnerToken = "owner" }},
		{"prepared timestamp", p, func(b *savedBatch) { b.UpdatedAt = timeText(now) }},
		{"prepared ambiguity", p, func(b *savedBatch) { b.Ambiguous = true }},
		{"unknown state", p, func(b *savedBatch) { b.State = "UNKNOWN" }},
		{"missing attempt", s, func(b *savedBatch) { b.AttemptCount = 0 }},
		{"missing time", s, func(b *savedBatch) { b.FirstAttemptAt = nil }},
		{"malformed time", s, func(b *savedBatch) { bad := "not-time"; b.FirstAttemptAt = &bad }},
		{"noncanonical time", s, func(b *savedBatch) { bad := "2026-01-01T00:00:00Z"; b.FirstAttemptAt = &bad }},
		{"year zero", s, func(b *savedBatch) { b.FirstAttemptAt = timeText(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)) }},
		{"zero time", s, func(b *savedBatch) { b.FirstAttemptAt = timeText(time.Time{}) }},
		{"bad owner", s, func(b *savedBatch) { b.OwnerToken = "/private" }},
		{"first ordering", s, func(b *savedBatch) { b.FirstAttemptAt = timeText(now.Add(time.Second)) }},
		{"count one ordering", s, func(b *savedBatch) { b.FirstAttemptAt = timeText(now.Add(-time.Second)) }},
		{"expired begin", s, func(b *savedBatch) { b.AttemptCount = 2; b.FirstAttemptAt = timeText(now.Add(-24 * time.Hour)) }},
		{"expiry equal", s, func(b *savedBatch) { b.AttemptExpiresAt = b.LastAttemptAt }},
		{"expiry long", s, func(b *savedBatch) { b.AttemptExpiresAt = timeText(now.Add(601 * time.Second)) }},
		{"updated early", s, func(b *savedBatch) { b.UpdatedAt = timeText(now.Add(-time.Second)) }},
		{"sending updated", s, func(b *savedBatch) { b.UpdatedAt = timeText(now.Add(time.Second)) }},
		{"sending reason", s, func(b *savedBatch) { b.Reason = ReasonNetwork }},
		{"sending next", s, func(b *savedBatch) { b.NextAttemptAt = timeText(now) }},
		{"first ambiguous", s, func(b *savedBatch) { b.Ambiguous = true }},
		{"acceptance absent", accepted, func(b *savedBatch) { b.AcceptedAt = nil }},
		{"acceptance differs", accepted, func(b *savedBatch) { b.AcceptedAt = timeText(now.Add(time.Second)) }},
		{"acceptance expired", accepted, func(b *savedBatch) { b.UpdatedAt = b.AttemptExpiresAt; b.AcceptedAt = b.UpdatedAt }},
		{"retry absent", wait, func(b *savedBatch) { b.NextAttemptAt = nil }},
		{"retry early", wait, func(b *savedBatch) { b.NextAttemptAt = timeText(now.Add(-time.Second)) }},
		{"retry horizon", wait, func(b *savedBatch) { b.NextAttemptAt = timeText(now.Add(24 * time.Hour)) }},
		{"retry reason", wait, func(b *savedBatch) { b.Reason = ReasonOperator }},
		{"network not ambiguous", wait, func(b *savedBatch) { b.Reason = ReasonNetwork }},
		{"expired too early", wait, func(b *savedBatch) { b.Reason = ReasonExpired; b.Ambiguous = true }},
		{"transient too late", wait, func(b *savedBatch) { b.UpdatedAt = b.AttemptExpiresAt; b.NextAttemptAt = b.UpdatedAt }},
		{"hold no updated", held, func(b *savedBatch) { b.UpdatedAt = nil }},
		{"hold unknown reason", held, func(b *savedBatch) { b.Reason = "private error" }},
		{"hold no-attempt permanent", held, func(b *savedBatch) { b.Reason = ReasonPermanent }},
	} {
		t.Run(tc.name, func(t *testing.T) { d := tc.base.copy(); tc.change(&d.Batches[0]); rejectedData(t, d) })
	}
}

func TestAttemptCountCannotWrapOnRestoredRecord(t *testing.T) {
	r, _ := codecFixture(t, "sample")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := r.data.Batches[0]
	r, ref, err := r.Begin(b.Message.BatchID, AttemptContext{"owner", b.Message.RoutingGeneration, now, now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Finish(ref, 503, now, now)
	if err != nil {
		t.Fatal(err)
	}
	d := r.copy()
	d.Batches[0].AttemptCount = math.MaxUint64
	r, err = decodeData(d)
	if err != nil {
		t.Fatal(err)
	}
	got, ref, err := r.Begin(b.Message.BatchID, AttemptContext{"owner", b.Message.RoutingGeneration, now, now.Add(time.Minute)})
	if err == nil || got.data != nil || ref != (AttemptRef{}) {
		t.Fatal("attempt counter wrapped")
	}
}

func TestDecodeChecksClosedPayloadAfterDigestIsRecomputed(t *testing.T) {
	r, _ := codecFixture(t, "sample")
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `"event_action":"trigger"`, `"event_action":"resolve"`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `"payload":{`, `"routing_key":"synthetic-value","payload":{`, 1)
		},
		func(s string) string { return strings.Replace(s, `"slot":1`, `"slot":3`, 1) },
		func(s string) string {
			return strings.Replace(s, `"source":"oci-maintenance-router/example"`, `"source":"oci-maintenance-router/invalid name"`, 1)
		},
		func(s string) string { return strings.Replace(s, `"value":null`, `"value":"invalid-time"`, 1) },
		func(s string) string { return strings.Replace(s, `"value":null`, `"value":"2026-01-01T00:00:00Z"`, 1) },
		func(s string) string {
			return strings.Replace(s, `"reference":"sample"`, `"reference":null,"Reference":"sample"`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `"resourceId":"ocid1.mysqldbsystem.oc1.iad.example"`, `"resourceId":"ocid1.mysqldbsystem.oc1.iad.other"`, 1)
		},
		func(s string) string { return "{" },
	} {
		d := r.copy()
		d.Batches[0].Message.Payload = change(d.Batches[0].Message.Payload)
		d.Batches[0].Message.PayloadDigest = digest([]byte(d.Batches[0].Message.Payload))
		rejectedData(t, d)
	}
}
