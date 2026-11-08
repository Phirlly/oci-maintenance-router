package delivery_test

import (
	"bytes"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestDraftOwnsEveryReturnedSlice(t *testing.T) {
	b, n := builder(t, nil), notice(t, record(db))
	m := map[string]routing.Lookup{db: metadata(db, "payments")}
	ids := []string{db}
	got := build(t, b, n, m, ids...)
	want := single(t, got)
	changed := got.Drafts[0].Snapshot()
	changed.Resources[0].ResourceID = "changed"
	changed.PayloadWithoutRoutingKey[0] = 'x'
	changed.BatchID, changed.SecretID = "changed", "changed"
	ids[0] = "changed"
	m[db].Metadata.DefinedTags["Operations"]["Owner"] = "video"
	if !reflect.DeepEqual(got.Drafts[0].Snapshot(), want) {
		t.Fatal("draft retained caller-owned data")
	}
	if zero := (delivery.Draft{}).Snapshot(); zero.BatchID != "" || len(zero.Resources) != 0 || len(zero.PayloadWithoutRoutingKey) != 0 {
		t.Fatal("zero draft appears prepared")
	}
}

func TestContextChangeDoesNotRewriteAnEarlierDraft(t *testing.T) {
	b, r := builder(t, nil), record(db)
	m := map[string]routing.Lookup{db: metadata(db, "payments")}
	before := single(t, build(t, b, notice(t, r), m, db))
	r.TimeUpdated = text("2026-02-01T00:00:00Z")
	updated := single(t, build(t, b, notice(t, r), m, db))
	renamed := single(t, build(t, builder(t, func(c map[string]any) { c["environmentName"] = "renamed" }), notice(t, record(db)), m, db))
	for _, after := range []delivery.Snapshot{updated, renamed} {
		if before.BatchID != after.BatchID || before.Revision != after.Revision || before.RoutingGeneration != after.RoutingGeneration || before.PayloadDigest == after.PayloadDigest || bytes.Equal(before.PayloadWithoutRoutingKey, after.PayloadWithoutRoutingKey) {
			t.Fatal("batch identity was confused with new payload bytes")
		}
	}
	// Reading an earlier value still has the original bytes; persistence must
	// select those first bytes for retries, never reconstruct them with Build.
	if envelope(t, before)["payload"].(map[string]any)["source"] != "oci-maintenance-router/example" {
		t.Fatal("old draft changed")
	}
}
