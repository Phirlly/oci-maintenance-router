package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
)

// CloseRecovery proposes terminal closure without sending or claiming acceptance.
func (r Record) CloseRecovery(req RecoveryRequest, at time.Time) (Record, RecoveryAction, error) {
	i, digest, err := r.recoveryLookup(req)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	if req.Kind != CloseBatch && req.Kind != CloseRevision {
		return Record{}, RecoveryAction{}, &Error{"invalid_recovery_kind"}
	}
	if i >= 0 {
		return r, copyRecoveryAction(r.data.Recovery.Actions[i]), nil
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	ids, reason := closeTargets(r.data, req)
	if reason != "" {
		return r.RejectRecovery(req, reason, at)
	}
	hash, err := r.ReviewFingerprint(req.Kind, req.TargetID, req.RoutingGeneration, "")
	if err != nil || hash != req.ReviewFingerprint {
		return r.RejectRecovery(req, "stale_review", at)
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	for _, id := range ids {
		d.delivery, err = d.delivery.Close(id, at)
		if err != nil {
			return Record{}, RecoveryAction{}, err
		}
	}
	a := RecoveryAction{Request: copyRecoveryRequest(req), IntentDigest: digest, RecordedAt: stamp, Status: RecoveryClosed, CompletedAt: &stamp, BatchIDs: []string{}}
	d.Recovery.Actions = append(d.Recovery.Actions, a)
	sortRecovery(d.Recovery)
	next, err := r.finalize(d, at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	return next, copyRecoveryAction(a), nil
}

func closeTargets(d *recordData, req RecoveryRequest) ([]string, string) {
	if req.Kind == CloseBatch {
		b, ok := d.delivery.Batch(req.TargetID)
		if !ok {
			return nil, "target_missing"
		}
		if b.State == delivery.Sending {
			return nil, "active_attempt"
		}
		if b.State == delivery.Closed || b.State == delivery.APIAccepted {
			return nil, "target_terminal"
		}
		return []string{req.TargetID}, ""
	}
	i := revisionIndex(d, req.TargetID)
	if i < 0 {
		return nil, "target_missing"
	}
	if recoveryRevisionClosed(d, req.TargetID) {
		return nil, "target_terminal"
	}
	ids := []string{}
	for _, b := range d.delivery.Batches() {
		if b.Message.Revision.Digest != req.TargetID {
			continue
		}
		if b.State == delivery.Sending {
			return nil, "active_attempt"
		}
		if b.State != delivery.APIAccepted && b.State != delivery.Closed {
			ids = append(ids, b.Message.BatchID)
		}
	}
	unfinished := len(d.Revisions[i].Resources) == 0
	for _, row := range d.Revisions[i].Resources {
		unfinished = unfinished || row.BatchID == ""
	}
	if len(ids) == 0 && !unfinished {
		return nil, "target_terminal"
	}
	return ids, ""
}
