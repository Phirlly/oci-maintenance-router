package delivery

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestAnnouncementRecoveryAndAssignmentsPreserveSavedMessage(t *testing.T) {
	r := announcementRecord(t, "ACTION_REQUIRED", "sample", announcementInstance, announcementDB)
	original := r.Batches()[0]
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	context := AttemptContext{OwnerToken: "owner", RoutingGeneration: original.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)}
	s, ref, err := r.Begin(original.Message.BatchID, context)
	if err != nil {
		t.Fatal(err)
	}
	if s.Batches()[0].AttemptKind != AutomaticAttempt {
		t.Fatal("automatic provenance lost")
	}
	if _, err := decodeData(s.copy()); err != nil {
		t.Fatal(err)
	}
	retry, err := s.Finish(ref, 503, at, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeData(retry.copy()); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{202, 0, 429, 500, 400} {
		held, err := retry.Hold(ref.BatchID, ReasonOperator, at)
		if err != nil {
			t.Fatal(err)
		}
		manual, mref, err := held.BeginManual(ref.BatchID, context)
		if err != nil {
			t.Fatal(err)
		}
		if manual.Batches()[0].AttemptKind != ManualAttempt {
			t.Fatal("manual provenance lost")
		}
		out, err := manual.Finish(mref, status, at, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		want := ReviewRequired
		if status == 202 {
			want = APIAccepted
		}
		if out.Batches()[0].State != want || !reflect.DeepEqual(out.Batches()[0].Message, original.Message) || out.Batches()[0].SecretVersion != 7 {
			t.Fatal("manual outcome/message changed")
		}
		if _, err := decodeData(out.copy()); err != nil {
			t.Fatal(err)
		}
		if status != 202 {
			out, err = out.Close(ref.BatchID, at)
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := decodeData(out.copy()); err != nil {
			t.Fatal(err)
		}
		ids, err := out.Unassigned(original.Message.Revision, []string{announcementInstance, announcementDB, announcementInstance + "new"})
		if err != nil || !reflect.DeepEqual(ids, []string{announcementInstance + "new"}) {
			t.Fatal("accepted/closed assignment lost", err)
		}
		if got, _, err := out.Begin(ref.BatchID, context); err == nil || got.data != nil {
			t.Fatal("terminal batch resent")
		}
	}
	// Closure without an attempt remains representable too.
	closed, err := r.Close(ref.BatchID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeData(closed.copy()); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{announcementInstance, announcementInstance}, {"ocid1.volume.oc1.iad.other"}} {
		if _, err := r.Unassigned(original.Message.Revision, ids); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
}

func TestAnnouncementPreparationKeepsMixedFormatBatchesImmutable(t *testing.T) {
	old, legacy := codecFixture(t, "legacy")
	old, err := old.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	old, err = old.MigrateAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	modern := announcementPreparation(t, "ACTION_REQUIRED", "new", announcementInstance)
	more, err := old.Prepare(modern)
	if err != nil || more.SchemaVersion() != 3 {
		t.Fatal(err)
	}
	if _, err := decodeData(more.copy()); err != nil {
		t.Fatal(err)
	}
	if len(more.Batches()) != 2 {
		t.Fatal("legacy batch lost")
	}
	before, _ := more.Encode()
	legacy.SecretVersion = 99
	modern.SecretVersion = 99
	again, err := more.Prepare(legacy, modern)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := again.Encode()
	if !bytes.Equal(before, after) {
		t.Fatal("first bytes/pin changed")
	}
	// NewRecord also supports mixed formats without changing legacy messages.
	mixed, err := NewRecord(legacy, modern)
	if err != nil || mixed.SchemaVersion() != 3 {
		t.Fatal(err)
	}
	if _, err := decodeData(mixed.copy()); err != nil {
		t.Fatal(err)
	}
}
