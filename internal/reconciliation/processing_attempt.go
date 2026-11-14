package reconciliation

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
)

func (r *processingRun) attempt(work processingNoticeWork, saved ocistorage.StoredNotice, id string) (ocistorage.StoredNotice, error) {
	for tries := 0; ; tries++ {
		b, ok := processingBatch(saved, id)
		if !ok {
			return saved, &ProcessingError{Code: "batch_missing"}
		}
		if b.State == delivery.APIAccepted || b.State == delivery.Closed {
			return saved, nil
		}
		if b.State == delivery.ReviewRequired {
			return saved, r.issue(ProcessingIssue{NoticeID: work.id, RevisionDigest: b.Message.Revision.Digest, BatchID: id, Code: string(b.Reason)})
		}
		if b.State == delivery.Sending || tries == 3 {
			r.result.snapshot.Deferred++
			return saved, nil
		}
		if b.State == delivery.RetryWait {
			deferRetry, err := r.awaitRetry(b)
			if err != nil {
				return saved, err
			}
			if deferRetry {
				r.result.snapshot.Deferred++
				return saved, nil
			}
			next, err := saved.Record().Recover(r.lease.manager.now())
			if err != nil {
				return saved, &ProcessingError{Code: "recovery_failed"}
			}
			saved, err = r.save(saved, next)
			if err != nil {
				return saved, err
			}
			b, _ = processingBatch(saved, id)
			if b.State == delivery.ReviewRequired {
				return saved, r.issue(ProcessingIssue{NoticeID: work.id, BatchID: id, Code: string(b.Reason)})
			}
		}
		selected, err := r.owner.selector.Select(r.ctx, r.lease)
		if err != nil {
			return saved, err
		}
		want, _ := r.baseline.Encode()
		got, _ := selected.Encode()
		if !bytes.Equal(want, got) {
			return saved, &ProcessingError{Code: "baseline_changed"}
		}
		refreshed, n, lookups, available, err := r.fresh(work, saved)
		if err != nil || !available {
			return refreshed, err
		}
		saved = refreshed
		b, _ = processingBatch(saved, id)
		if b.State == delivery.ReviewRequired {
			return saved, r.issue(ProcessingIssue{NoticeID: work.id, BatchID: id, Code: string(b.Reason)})
		}
		plan := r.owner.policy.Resolve(n, true, lookups)
		_, approved := processingApproval(saved, id)
		if !r.batchEligible(b, plan, lookups, approved) {
			return r.holdBatch(saved, b, delivery.ReasonOperator, "ownership_changed")
		}
		if err = r.before(); err != nil {
			return saved, err
		}
		credential, err := r.owner.version(r.ctx, b.Message.SecretID, b.SecretVersion)
		if r.ctx.Err() != nil {
			return saved, processingFailure(r.ctx, nil)
		}
		if err != nil || credential == nil {
			return r.holdBatch(saved, b, delivery.ReasonSecretUnavailable, "secret_unavailable")
		}
		key := credential.Bytes()
		if credential.Version() != b.SecretVersion || !pagerduty.ValidRoutingKey(key) {
			clear(key)
			return r.holdBatch(saved, b, delivery.ReasonSecretUnavailable, "secret_invalid")
		}
		if approved {
			saved, err = r.claimApproved(saved, b, key, work)
		} else {
			saved, err = r.claimAndSend(saved, b, key)
		}
		clear(key)
		if err != nil {
			return saved, err
		}
	}
}

func (r *processingRun) batchEligible(b delivery.BatchSnapshot, plan routing.Plan, lookups map[string]routing.Lookup, approved bool) bool {
	if plan.Assessment.State != routing.SupportedShape || plan.Revision != b.Message.Revision || plan.RoutingGeneration != b.Message.RoutingGeneration {
		return false
	}
	decisions := map[string]routing.Decision{}
	for _, d := range plan.Decisions {
		decisions[d.ResourceID] = d
	}
	for _, member := range b.Message.Resources {
		d, ok := decisions[member.ResourceID]
		if !ok || d.Disposition != routing.Ready || d.Region != member.Region || d.DestinationID != b.Message.DestinationID || (!approved && r.suppressed(plan.Revision, d, lookups)) {
			return false
		}
	}
	return true
}

func (r *processingRun) awaitRetry(b delivery.BatchSnapshot) (bool, error) {
	now := r.lease.manager.now()
	if !leaseWallTime(now) {
		return false, &ProcessingError{Code: "invalid_time"}
	}
	deadline, _ := r.ctx.Deadline()
	if !b.NextAttemptAt.Add(discoveryReserve).Before(deadline) {
		return true, nil
	}
	if delay := b.NextAttemptAt.Sub(now); delay > 0 {
		if err := r.owner.wait(r.ctx, delay); err != nil {
			return false, processingFailure(r.ctx, &ProcessingError{Code: "wait_failed"})
		}
	}
	return false, r.reserve()
}

func (r *processingRun) claimAndSend(saved ocistorage.StoredNotice, b delivery.BatchSnapshot, key []byte) (ocistorage.StoredNotice, error) {
	if err := r.reportCapacity(len(b.Message.Revision.NoticeID) + len(b.Message.BatchID)); err != nil {
		return saved, err
	}
	if err := r.guard(); err != nil {
		return saved, err
	}
	expires, _ := leaseTimestamp(r.lease.record.ExpiresAt)
	next, ref, err := saved.Record().Begin(b.Message.BatchID, delivery.AttemptContext{OwnerToken: r.token, RoutingGeneration: b.Message.RoutingGeneration, Now: r.lease.manager.now(), LeaseExpiresAt: expires})
	if err != nil {
		return saved, &ProcessingError{Code: "attempt_failed"}
	}
	return r.sendClaim(saved, b, next, ref, key)
}

func (r *processingRun) sendClaim(saved ocistorage.StoredNotice, b delivery.BatchSnapshot, next noticeledger.Record, ref delivery.AttemptRef, key []byte) (ocistorage.StoredNotice, error) {
	saved, err := r.save(saved, next)
	if err != nil {
		return saved, err
	}
	if err = r.before(); err != nil {
		return saved, err
	}
	if err = r.guard(); err != nil {
		return saved, err
	}
	return r.dispatch(saved, b, ref, key)
}
