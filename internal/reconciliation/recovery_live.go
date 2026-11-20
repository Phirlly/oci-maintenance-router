package reconciliation

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func (r *processingRun) recoverLive(works []processingNoticeWork) error {
	if err := r.guard(); err != nil {
		return err
	}
	q := r.recoveryRequest()
	saved, err := r.store.Load(r.ctx, r.owner.config.Recovery.NoticeID)
	if err != nil {
		return err
	}
	_, exists, err := saved.Record().LookupRecovery(q)
	if err != nil {
		return &ProcessingError{Code: "recovery_intent_conflict"}
	}
	if exists {
		r.captureRecovery(saved)
	}
	var selected processingNoticeWork
	for _, work := range works {
		if work.id == r.owner.config.Recovery.NoticeID {
			selected = work
		}
	}
	if savedMembership(saved.Record()) != selected.membership {
		return &ProcessingError{Code: "membership_changed"}
	}
	if exists {
		return r.guard()
	}
	digest, reason := recoveryTarget(saved.Record(), q)
	if reason != "" {
		return r.rejectRecovery(saved, reason)
	}
	if !selected.stable || selected.component != q.ComponentFingerprint {
		return r.rejectRecovery(saved, "component_changed")
	}
	if !selected.available {
		return r.rejectRecovery(saved, "source_unavailable")
	}
	found := false
	for _, member := range selected.members {
		if member.Revision.Digest == digest && selected.current[member.Revision] {
			selected.member, found = member, true
			break
		}
	}
	if !found {
		return r.rejectRecovery(saved, "source_changed")
	}
	saved, n, lookups, available, err := r.fresh(selected, saved)
	if err != nil {
		if e, ok := err.(*ProcessingError); ok && e.Code == "source_changed" {
			if failure := r.rejectCurrentRecovery("source_changed"); failure != nil {
				return failure
			}
		}
		return err
	}
	if !available {
		return r.rejectRecovery(saved, "source_unavailable")
	}
	hash, err := saved.Record().ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, q.ComponentFingerprint)
	if err != nil || hash != q.ReviewFingerprint {
		return r.rejectRecovery(saved, "stale_review")
	}
	if q.Kind == noticeledger.RetryBatch {
		return r.retryRecovery(saved, n, lookups)
	}
	return r.approveRecovery(saved, n, lookups)
}

func recoveryTarget(record noticeledger.Record, q noticeledger.RecoveryRequest) (string, string) {
	if q.Kind == noticeledger.RetryBatch {
		var b delivery.BatchSnapshot
		found := false
		for _, candidate := range record.Snapshot().Batches {
			if candidate.Message.BatchID == q.TargetID {
				b, found = candidate, true
			}
		}
		if !found {
			return "", "target_missing"
		}
		switch b.State {
		case delivery.Sending:
			return "", "active_attempt"
		case delivery.APIAccepted, delivery.Closed:
			return "", "target_terminal"
		case delivery.ReviewRequired:
			return b.Message.Revision.Digest, ""
		default:
			return "", "no_eligible_work"
		}
	}
	for _, action := range record.Snapshot().Recovery.Actions {
		if action.Status == noticeledger.RecoveryClosed && action.Request.Kind == noticeledger.CloseRevision && action.Request.TargetID == q.TargetID {
			return "", "target_terminal"
		}
	}
	for _, revision := range record.Snapshot().Revisions {
		if revision.Digest == q.TargetID {
			return q.TargetID, ""
		}
	}
	return "", "target_missing"
}

func (r *processingRun) rejectCurrentRecovery(reason string) error {
	if err := r.guard(); err != nil {
		return err
	}
	saved, err := r.store.Load(r.ctx, r.owner.config.Recovery.NoticeID)
	if err != nil {
		return err
	}
	return r.rejectRecovery(saved, reason)
}

func missingRecoveryReadiness(err error) bool {
	e, ok := err.(*LiveSelectionError)
	if !ok || e == nil || e.WriteUncertain || e.HTTPStatus != 0 {
		return false
	}
	switch e.Code {
	case "not_accepted", "receipt_mismatch", "channel_changed", "baseline_unprepared", "scope_unprepared", "confirmation_required", "setup_stale", "confirmation_mismatch":
		return true
	}
	return false
}

func (r *processingRun) recoveryReadiness() error {
	selected, err := r.owner.selector.Select(r.ctx, r.lease)
	if err != nil {
		return err
	}
	want, _ := r.baseline.Encode()
	got, _ := selected.Encode()
	if !bytes.Equal(want, got) {
		return &ProcessingError{Code: "baseline_changed"}
	}
	return nil
}
