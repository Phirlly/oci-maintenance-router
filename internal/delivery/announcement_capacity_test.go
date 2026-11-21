package delivery

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestAnnouncementMigrationAndPreparationAtExactCapacity(t *testing.T) {
	base := recoveryCapacityBase(t)
	base, err := base.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	_, tail := codecFixture(t, "tail")
	probe, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	room := (1 << 20) - recoveryMaximumMetadata(t, probe)
	_, tail = codecFixture(t, "tail"+strings.Repeat("x", room))
	full, err := base.Prepare(tail)
	if err != nil {
		t.Fatal(err)
	}
	next, err := full.MigrateAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	size, err := next.ReservedBytes()
	if err != nil || size != 1<<20 {
		t.Fatal("migration changed exact capacity", size, err)
	}
	if _, err := decodeData(next.copy()); err != nil {
		t.Fatal(err)
	}
	before, _ := next.Encode()
	addition := announcementPreparation(t, "ACTION_REQUIRED", "sample", announcementInstance)
	if got, err := next.Prepare(addition); err == nil || got.data != nil {
		t.Fatal("overflow admitted")
	}
	unchanged, _ := next.Encode()
	if !bytes.Equal(before, unchanged) {
		t.Fatal("failed addition mutated predecessor")
	}
	b := next.Batches()[0]
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, ref, err := next.Begin(b.Message.BatchID, AttemptContext{OwnerToken: strings.Repeat("a", 128), RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.Finish(ref, 202, at, time.Time{})
	if err != nil {
		t.Fatal("capacity lost receipt", err)
	}
	if _, err := decodeData(accepted.copy()); err != nil {
		t.Fatal(err)
	}
	expired, err := s.Recover(at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeData(expired.copy()); err != nil {
		t.Fatal(err)
	}
}
