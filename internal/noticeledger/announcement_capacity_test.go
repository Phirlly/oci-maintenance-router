package noticeledger

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestAnnouncementMigrationAtExactCapacityRetainsTerminalRoom(t *testing.T) {
	p := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
	s, ref := begin(t, p, p.Snapshot().Batches[0].Message.BatchID, clock())
	manual, _, manualRef, manualAt := claimedRetry(t, false)
	approved, _, _ := approvedRecord(t, true)
	for _, r := range []Record{recoveryRecord(t, observed(t, source(db), db)), s, manual, approved} {
		full := fullSourceRecord(t, r)
		next := migrateAnnouncements(t, full)
		if maximumSize(t, next) != 1<<20 {
			t.Fatal("reservation changed")
		}
		reload(t, next)
		before := encoded(t, next)
		env, id := next.Identity()
		over := changedWire(t, next, func(w *wireRecord) { *w.Revisions[0].Source.Reference += "x" })
		bad, decodeErr := Decode(over, env, id)
		rejected(t, bad, decodeErr)
		var finished Record
		var err error
		switch {
		case len(next.Snapshot().Batches) == 0:
			bad, _, actionErr := next.RejectRecovery(recoveryIntent(t), "readiness_required", clock().Add(time.Second))
			rejected(t, bad, actionErr)
			var detail *Error
			if !errors.As(actionErr, &detail) || detail.Code != "too_large" {
				t.Fatal("wrong action rejection", actionErr)
			}
			bad, prepareErr := next.Prepare(clock().Add(time.Second), preparations(t, source(db), db)...)
			rejected(t, bad, prepareErr)
			if !errors.As(prepareErr, &detail) || detail.Code != "too_large" {
				t.Fatal("wrong preparation rejection", prepareErr)
			}
			finished, err = next.RecordSourceUnavailable(503, clock().Add(time.Second))
		case len(next.Snapshot().Recovery.Actions) == 0:
			finished, err = next.Finish(ref, 202, clock().Add(time.Second), time.Time{})
		case next.Snapshot().Recovery.Actions[0].Status == RecoveryConsumed:
			finished, err = next.Finish(manualRef, 503, manualAt.Add(time.Second), time.Time{})
		default:
			finished, err = next.Hold(delivery.ReasonConfigurationChanged, clock().Add(time.Second))
		}
		if err != nil {
			t.Fatal("reserved outcome cannot fit", err)
		}
		reload(t, finished)
		if !bytes.Equal(before, encoded(t, next)) {
			t.Fatal("capacity/outcome mutated predecessor")
		}
	}
}
