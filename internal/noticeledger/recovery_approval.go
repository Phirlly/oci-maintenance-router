package noticeledger

import (
	"slices"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// ApproveRecovery proposes a one-shot approval for exact newly prepared work.
func (o *Observer) ApproveRecovery(r Record, req RecoveryRequest, n notices.Notice, knownCandidate bool, lookups map[string]routing.Lookup, at time.Time, values ...delivery.Preparation) (Record, RecoveryAction, error) {
	i, digest, err := r.recoveryLookup(req)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	if req.Kind != ApproveRevision {
		return Record{}, RecoveryAction{}, &Error{"invalid_recovery_kind"}
	}
	if i >= 0 {
		return r, copyRecoveryAction(r.data.Recovery.Actions[i]), nil
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	if o == nil || o.policy == nil || o.environmentID != r.data.EnvironmentID || n.Key().NoticeID != r.data.NoticeID || n.Key().Digest != req.TargetID {
		return Record{}, RecoveryAction{}, &Error{"invalid_approval_context"}
	}
	i = revisionIndex(r.data, req.TargetID)
	if i < 0 {
		return r.RejectRecovery(req, "target_missing", at)
	}
	if recoveryRevisionClosed(r.data, req.TargetID) {
		return r.RejectRecovery(req, "target_terminal", at)
	}
	if reason := r.recoveryPrerequisite(req, req.TargetID); reason != "" {
		return r.RejectRecovery(req, reason, at)
	}
	plan := o.policy.Resolve(n, knownCandidate, lookups)
	if plan.RoutingGeneration != req.RoutingGeneration {
		return r.RejectRecovery(req, "configuration_changed", at)
	}
	if plan.Assessment.State != routing.SupportedShape || len(values) == 0 {
		return r.RejectRecovery(req, "no_eligible_work", at)
	}
	if len(values) > 256 {
		return Record{}, RecoveryAction{}, &Error{"too_many_preparations"}
	}
	current := projectRevision(n, plan)
	d := r.data.copy()
	ids, err := prepareApprovedRows(&d, i, current, values)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	d.UpdatedAt = stamp
	d.delivery, err = prepareDeliveries(d.delivery, 3, values)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	slices.Sort(ids)
	a := RecoveryAction{Request: copyRecoveryRequest(req), IntentDigest: digest, RecordedAt: stamp, Status: RecoveryApproved, CompletedAt: &stamp, BatchIDs: ids}
	d.Recovery.Actions = append(d.Recovery.Actions, a)
	sortRecovery(d.Recovery)
	next, err := r.finalize(d, at)
	if err != nil {
		return Record{}, RecoveryAction{}, err
	}
	return next, copyRecoveryAction(a), nil
}

// BeginApproved proposes an automatic attempt only for a retained approval member.
func (r Record) BeginApproved(requestID, batchID, component string, c delivery.AttemptContext) (Record, delivery.AttemptRef, error) {
	if r.data == nil || r.data.SchemaVersion != 3 {
		return Record{}, delivery.AttemptRef{}, &Error{"recovery_migration_required"}
	}
	a, ok := recoveryApproval(r.data, batchID)
	if !ok || a.Request.RequestID != requestID || a.Request.ComponentFingerprint != component || c.RoutingGeneration != a.Request.RoutingGeneration || !activeApproval(r.data, a) {
		return Record{}, delivery.AttemptRef{}, &Error{"approval_blocked"}
	}
	stamp, err := r.operationTime(c.Now)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	deliveryNext, ref, err := d.delivery.Begin(batchID, c)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	d.delivery = deliveryNext
	next, err := r.finalize(d, c.Now)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	return next, ref, nil
}
