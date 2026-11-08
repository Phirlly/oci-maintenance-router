package delivery_test

import (
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func preparations(t *testing.T, b *delivery.Builder, source notices.Record, selected ...string) []delivery.Preparation {
	t.Helper()
	m := map[string]routing.Lookup{}
	for _, id := range selected {
		m[id] = metadata(id, "payments")
	}
	result := build(t, b, notice(t, source), m, selected...)
	if len(result.Issues) != 0 {
		t.Fatal("unexpected capacity issue")
	}
	var values []delivery.Preparation
	for _, draft := range result.Drafts {
		values = append(values, delivery.Preparation{Draft: draft, SecretVersion: 7})
	}
	return values
}

func prepared(t *testing.T, ids ...string) (delivery.Record, []delivery.Preparation) {
	t.Helper()
	values := preparations(t, builder(t, nil), record(ids...), ids...)
	r, err := delivery.NewRecord(values...)
	if err != nil {
		t.Fatal(err)
	}
	return r, values
}

func instant() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func attempt(r delivery.Record, now time.Time) (delivery.Record, delivery.AttemptRef, error) {
	b := r.Batches()[0]
	return r.Begin(b.Message.BatchID, delivery.AttemptContext{OwnerToken: "invocation-1", RoutingGeneration: b.Message.RoutingGeneration, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
}

func sending(t *testing.T) (delivery.Record, delivery.AttemptRef) {
	t.Helper()
	r, _ := prepared(t, db)
	r, ref, err := attempt(r, instant())
	if err != nil {
		t.Fatal(err)
	}
	return r, ref
}

func rejectedRecord(t *testing.T, r delivery.Record, err error) {
	t.Helper()
	var safe *delivery.Error
	if !errors.As(err, &safe) || len(r.Batches()) != 0 {
		t.Fatalf("usable or uncontrolled failure: %v %v", r.Batches(), err)
	}
}
