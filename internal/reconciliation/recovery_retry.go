package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
)

func (r *processingRun) retryRecovery(saved ocistorage.StoredNotice, n notices.Notice, lookups map[string]routing.Lookup) error {
	b, _ := processingBatch(saved, r.recoveryRequest().TargetID)
	plan := r.owner.policy.Resolve(n, true, lookups)
	if reason := r.retryCompatibility(b, plan); reason != "" {
		return r.rejectRecovery(saved, reason)
	}
	if err := r.before(); err != nil {
		return err
	}
	credential, err := r.owner.version(r.ctx, b.Message.SecretID, b.SecretVersion)
	if r.ctx.Err() != nil {
		return processingFailure(r.ctx, nil)
	}
	if err != nil || credential == nil {
		return r.rejectRecovery(saved, "secret_unavailable")
	}
	key := credential.Bytes()
	defer clear(key)
	if credential.Version() != b.SecretVersion || !pagerduty.ValidRoutingKey(key) {
		return r.rejectRecovery(saved, "secret_unavailable")
	}
	if err = r.recoveryReadiness(); err != nil {
		if missingRecoveryReadiness(err) {
			if failure := r.rejectRecovery(saved, "readiness_required"); failure != nil {
				return failure
			}
		}
		return err
	}
	if err = r.reportCapacity(len(b.Message.Revision.NoticeID) + len(b.Message.BatchID)); err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	expires, _ := leaseTimestamp(r.lease.record.ExpiresAt)
	next, _, ref, err := saved.Record().RetryRecovery(r.recoveryRequest(), delivery.AttemptContext{
		OwnerToken: r.token, RoutingGeneration: b.Message.RoutingGeneration, Now: r.lease.manager.now(), LeaseExpiresAt: expires})
	if err != nil {
		return &ProcessingError{Code: "recovery_failed"}
	}
	if ref.BatchID == "" {
		_, err = r.save(saved, next)
		return err
	}
	_, err = r.sendClaim(saved, b, next, ref, key)
	return err
}

func (r *processingRun) retryCompatibility(b delivery.BatchSnapshot, plan routing.Plan) string {
	if plan.Assessment.State != routing.SupportedShape || plan.Revision != b.Message.Revision {
		return "source_changed"
	}
	secret := ""
	for _, d := range r.owner.config.Destinations {
		if d.ID == b.Message.DestinationID {
			secret = d.SecretID
		}
	}
	if secret != b.Message.SecretID || r.owner.config.PagerDutyRegion != b.Message.PagerDutyRegion {
		return "configuration_changed"
	}
	decisions := map[string]routing.Decision{}
	for _, d := range plan.Decisions {
		decisions[d.ResourceID] = d
	}
	for _, member := range b.Message.Resources {
		d, ok := decisions[member.ResourceID]
		if !ok || d.Disposition != routing.Ready || d.Region != member.Region || d.DestinationID != b.Message.DestinationID {
			return "ownership_changed"
		}
	}
	return ""
}
