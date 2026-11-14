package noticeledger

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryClosureCannotPredateTargetObservation(t *testing.T) {
	r := retryReady(t, false)
	q := closeRequest(t, r, CloseBatch, r.Snapshot().Batches[0].Message.BatchID)
	closed, _, err := r.CloseRecovery(q, clock())
	if err != nil {
		t.Fatal(err)
	}
	earlier := clock().Add(-time.Second).Format(timeFormat)
	env, id := closed.Identity()
	body := changedWire(t, closed, func(w *wireRecord) {
		a := &w.Recovery.Actions[0]
		a.RecordedAt = earlier
		a.CompletedAt = &earlier
		w.Delivery = bytes.Replace(w.Delivery, []byte(`"updatedAt":"`+clock().Format(timeFormat)+`"`), []byte(`"updatedAt":"`+earlier+`"`), 1)
	})
	bad, err := Decode(body, env, id)
	rejected(t, bad, err)
}

func TestRecoveryClosureCodecRejectsMissingCoverageAndForgedResults(t *testing.T) {
	r := retryReady(t, true)
	q := closeRequest(t, r, CloseBatch, r.Snapshot().Batches[0].Message.BatchID)
	closed, _, err := r.CloseRecovery(q, clock())
	if err != nil {
		t.Fatal(err)
	}
	rejectRecoveryWire(t, closed, map[string]func(*wireRecord){
		"missing_action":     func(w *wireRecord) { w.Recovery.Actions = []RecoveryAction{} },
		"extra_batch_list":   func(w *wireRecord) { w.Recovery.Actions[0].BatchIDs = []string{q.TargetID} },
		"unknown_result":     func(w *wireRecord) { w.Recovery.Actions[0].Status = RecoveryApproved },
		"missing_completion": func(w *wireRecord) { w.Recovery.Actions[0].CompletedAt = nil },
		"claimed_acceptance": func(w *wireRecord) { w.Revisions[0].Resources[0].Disposition = "API_ACCEPTED" },
		"lost_assignment":    func(w *wireRecord) { w.Revisions[0].Resources[0].BatchID = "" },
	})
}

func TestRecoveryClosureCodecRejectsSuccessWithNoUnfinishedWork(t *testing.T) {
	r := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
	id := r.Snapshot().Batches[0].Message.BatchID
	sending, ref, err := r.Begin(id, approvedContext(r, id, clock()))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := sending.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q := closeRequest(t, accepted, CloseRevision, accepted.Snapshot().Revisions[0].Digest)
	rejectedRecord, a, err := accepted.CloseRecovery(q, clock())
	if err != nil || a.Status != RecoveryRejected || a.Reason != "target_terminal" || rejectedRecord.Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("completed revision was closed", err)
	}
	stamp := clock().Format(timeFormat)
	env, notice := accepted.Identity()
	body := changedWire(t, accepted, func(w *wireRecord) {
		w.Recovery.Actions = []RecoveryAction{{Request: q, IntentDigest: recoveryIntentDigest(w.recordData, q), RecordedAt: stamp, Status: RecoveryClosed, CompletedAt: &stamp, BatchIDs: []string{}}}
	})
	bad, err := Decode(body, env, notice)
	rejected(t, bad, err)
}
