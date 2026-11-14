package delivery

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// Independent explicit worst-case wire fields; no production reservation helper.
func recoveryMaximumMetadata(t *testing.T, r Record) int {
	t.Helper()
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err = json.Unmarshal(body, &w); err != nil {
		t.Fatal(err)
	}
	w["schemaVersion"] = 2
	stamp := "9999-12-31T23:59:59.999Z"
	for _, raw := range w["batches"].([]any) {
		b := raw.(map[string]any)
		b["state"], b["reason"], b["attemptKind"] = "REVIEW_REQUIRED", "configuration_changed", "AUTOMATIC"
		b["attemptCount"], b["ownerToken"], b["ambiguous"] = uint64(math.MaxUint64), strings.Repeat("a", 128), false
		for _, field := range []string{"firstAttemptAt", "lastAttemptAt", "attemptExpiresAt", "updatedAt", "nextAttemptAt", "acceptedAt"} {
			b[field] = stamp
		}
	}
	body, err = json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return len(body)
}

func recoveryCapacityBase(t *testing.T) Record {
	t.Helper()
	var values []Preparation
	for i := 0; i < 32; i++ {
		_, p := codecFixture(t, fmt.Sprintf("%02d", i)+strings.Repeat("x", 30000))
		values = append(values, p)
	}
	r, err := NewRecord(values...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRecoveryCapacityReservesExactKindHeadroom(t *testing.T) {
	base := recoveryCapacityBase(t)
	_, tail := codecFixture(t, "tail")
	probe, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	room := (1 << 20) - recoveryMaximumMetadata(t, probe)
	if room < 0 || room > 30000 {
		t.Fatal("invalid fixture headroom", room)
	}
	_, tail = codecFixture(t, "tail"+strings.Repeat("x", room))
	full, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	full, err = full.MigrateRecovery()
	if err != nil {
		t.Fatal("exact v2 boundary rejected", err)
	}
	if recoveryMaximumMetadata(t, full) != 1<<20 {
		t.Fatal("incorrect exact boundary")
	}
	if _, err := decodeData(full.copy()); err != nil {
		t.Fatal(err)
	}
	// Exercise manual outcomes and closure at the exact admission limit.
	b := full.Batches()[0]
	at := time.Date(9999, 12, 30, 0, 0, 0, 0, time.UTC)
	working, err := full.Hold(b.Message.BatchID, ReasonOperator, at)
	if err != nil {
		t.Fatal(err)
	}
	working, ref, err := working.BeginManual(b.Message.BatchID, AttemptContext{OwnerToken: strings.Repeat("a", 128), RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(600 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	working, err = working.Finish(ref, 0, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	working, err = working.Close(b.Message.BatchID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeData(working.copy()); err != nil {
		t.Fatal("reserved metadata did not cover closure", err)
	}
	if recoveryMaximumMetadata(t, working) != 1<<20 {
		t.Fatal("outcomes changed reserved maximum")
	}
	_, over := codecFixture(t, "tail"+strings.Repeat("x", room+1))
	tooLarge, err := base.Prepare(over)
	if err != nil {
		t.Fatal("v1 precursor should still fit", err)
	}
	got, err := tooLarge.MigrateRecovery()
	if err == nil || got.data != nil {
		t.Fatal("v2 one-byte overflow admitted")
	}
	base2, err := base.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	got, err = base2.Prepare(over)
	if err == nil || got.data != nil {
		t.Fatal("v2 preparation skipped kind reservation")
	}
}

func TestFailedRecoveryMigrationPreservesLegacyLateOutcomeAndExpiry(t *testing.T) {
	base := recoveryCapacityBase(t)
	_, tail := codecFixture(t, "tail")
	probe, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	room := (1 << 20) - maximumMetadata(probe.copy())
	_, tail = codecFixture(t, "tail"+strings.Repeat("x", room))
	full, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	if maximumMetadata(full.copy()) != 1<<20 {
		t.Fatal("legacy boundary not reached")
	}
	b := full.Batches()[0]
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, ref, err := full.Begin(b.Message.BatchID, AttemptContext{OwnerToken: "old", RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.MigrateRecovery()
	if err == nil || got.data != nil {
		t.Fatal("oversize migration admitted")
	}
	accepted, err := s.Finish(ref, 202, at.Add(time.Second), time.Time{})
	if err != nil {
		t.Fatal("legacy receipt blocked", err)
	}
	if accepted.SchemaVersion() != 1 {
		t.Fatal("receipt implicitly migrated")
	}
	if _, err := decodeData(accepted.copy()); err != nil {
		t.Fatal(err)
	}
	expired, err := s.Recover(at.Add(time.Minute))
	if err != nil {
		t.Fatal("legacy recovery blocked", err)
	}
	if _, err := decodeData(expired.copy()); err != nil {
		t.Fatal(err)
	}
	old, ok := s.Batch(b.Message.BatchID)
	if !ok || old.State != Sending {
		t.Fatal("failed migration mutated predecessor")
	}
}
