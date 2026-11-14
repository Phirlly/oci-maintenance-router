package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
)

// RetryRecovery consumes an explicit request together with one proposed manual
// attempt. Only a newly confirmed, still-authorized claim can later dispatch.
func (r Record) RetryRecovery(req RecoveryRequest, c delivery.AttemptContext) (Record, RecoveryAction, delivery.AttemptRef, error) {
	i, digest, err := r.recoveryLookup(req)
	if err != nil {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, err
	}
	if req.Kind != RetryBatch {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, &Error{"invalid_recovery_kind"}
	}
	if i >= 0 {
		return r, copyRecoveryAction(r.data.Recovery.Actions[i]), delivery.AttemptRef{}, nil
	}
	stamp, err := r.operationTime(c.Now)
	if err != nil {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, err
	}
	if !validRecoveryAttemptContext(c) {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, &Error{"invalid_recovery_attempt"}
	}
	b, found := r.data.delivery.Batch(req.TargetID)
	if found && c.RoutingGeneration != b.Message.RoutingGeneration {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, &Error{"invalid_recovery_attempt"}
	}
	reason := ""
	switch {
	case !found:
		reason = "target_missing"
	case b.State == delivery.Sending:
		reason = "active_attempt"
	case b.State == delivery.APIAccepted || b.State == delivery.Closed:
		reason = "target_terminal"
	case b.State != delivery.ReviewRequired:
		reason = "no_eligible_work"
	}
	if reason != "" {
		next, a, err := r.RejectRecovery(req, reason, c.Now)
		return next, a, delivery.AttemptRef{}, err
	}
	started, ref, err := r.data.delivery.BeginManual(req.TargetID, c)
	if err != nil {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, err
	}
	if reason = r.recoveryPrerequisite(req, b.Message.Revision.Digest); reason != "" {
		next, a, err := r.RejectRecovery(req, reason, c.Now)
		return next, a, delivery.AttemptRef{}, err
	}
	claimed, _ := started.Batch(req.TargetID)
	a := RecoveryAction{Request: copyRecoveryRequest(req), IntentDigest: digest, RecordedAt: stamp,
		Status: RecoveryConsumed, BatchIDs: []string{}, Ambiguous: claimed.Ambiguous,
		Attempt: &RecoveryAttempt{BatchID: ref.BatchID, OwnerToken: ref.OwnerToken, AttemptCount: ref.AttemptCount, ExpiresAt: reviewTime(claimed.AttemptExpiresAt)}}
	d := r.data.copy()
	d.UpdatedAt, d.delivery = stamp, started
	d.Recovery.Actions = append(d.Recovery.Actions, a)
	sortRecovery(d.Recovery)
	next, err := r.finalize(d, c.Now)
	if err != nil {
		return Record{}, RecoveryAction{}, delivery.AttemptRef{}, err
	}
	return next, copyRecoveryAction(a), ref, nil
}

func validRecoveryAttemptContext(c delivery.AttemptContext) bool {
	now, expires := c.Now.UTC().Truncate(time.Millisecond), c.LeaseExpiresAt.UTC().Truncate(time.Millisecond)
	return recoveryID.MatchString(c.OwnerToken) && digestPattern.MatchString(c.RoutingGeneration) &&
		!expires.IsZero() && expires.Year() >= 1 && expires.Year() <= 9999 &&
		expires.After(now) && expires.Sub(now) <= 600*time.Second
}

func (r Record) recoveryPrerequisite(req RecoveryRequest, revision string) string {
	hash, err := r.ReviewFingerprint(req.Kind, req.TargetID, req.RoutingGeneration, req.ComponentFingerprint)
	if err != nil || hash != req.ReviewFingerprint {
		return "stale_review"
	}
	o := r.data.SourceOutcome
	if o.State != SourceAvailable {
		return "source_unavailable"
	}
	if o.RevisionDigest != revision {
		return "source_changed"
	}
	i := revisionIndex(r.data, revision)
	if i < 0 || r.data.Revisions[i].RoutingGeneration != req.RoutingGeneration {
		return "configuration_changed"
	}
	return ""
}
