package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"slices"
)

func processingApproval(saved ocistorage.StoredNotice, id string) (noticeledger.RecoveryAction, bool) {
	if history := saved.Record().Snapshot().Recovery; history != nil {
		for _, a := range history.Actions {
			if a.Status == noticeledger.RecoveryApproved && slices.Contains(a.BatchIDs, id) {
				return a, true
			}
		}
	}
	return noticeledger.RecoveryAction{}, false
}

func approvalWork(work processingNoticeWork, saved ocistorage.StoredNotice, a noticeledger.RecoveryAction, b delivery.BatchSnapshot) (processingNoticeWork, bool) {
	o := saved.Record().Snapshot().SourceOutcome
	if a.InvalidatedAt != nil || !work.stable || !work.available || a.Request.ComponentFingerprint != work.component || o == nil || o.State != noticeledger.SourceAvailable || o.RevisionDigest != b.Message.Revision.Digest {
		return work, false
	}
	for _, member := range work.members {
		if member.Revision == b.Message.Revision && work.current[member.Revision] {
			work.member = member
			return work, true
		}
	}
	return work, false
}

func (r *processingRun) claimApproved(saved ocistorage.StoredNotice, b delivery.BatchSnapshot, key []byte, work processingNoticeWork) (ocistorage.StoredNotice, error) {
	if err := r.reportCapacity(len(b.Message.Revision.NoticeID) + len(b.Message.BatchID)); err != nil {
		return saved, err
	}
	if err := r.guard(); err != nil {
		return saved, err
	}
	a, ok := processingApproval(saved, b.Message.BatchID)
	if !ok {
		return saved, &ProcessingError{Code: "approval_changed"}
	}
	expires, _ := leaseTimestamp(r.lease.record.ExpiresAt)
	next, ref, err := saved.Record().BeginApproved(a.Request.RequestID, b.Message.BatchID, work.component,
		delivery.AttemptContext{OwnerToken: r.token, RoutingGeneration: b.Message.RoutingGeneration, Now: r.lease.manager.now(), LeaseExpiresAt: expires})
	if err != nil {
		return saved, &ProcessingError{Code: "approval_blocked"}
	}
	return r.sendClaim(saved, b, next, ref, key)
}
