package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
)

func (r *processingRun) process(work processingNoticeWork) error {
	if err := r.guard(); err != nil {
		return err
	}
	saved, err := r.store.Load(r.ctx, work.id)
	if err != nil {
		return err
	}
	if saved.Record().Snapshot().SchemaVersion == 3 {
		return &ProcessingError{Code: "unsupported_notice_schema"}
	}
	if savedMembership(saved.Record()) != work.membership {
		return &ProcessingError{Code: "membership_changed"}
	}
	next, err := saved.Record().Recover(r.lease.manager.now())
	if err != nil {
		return &ProcessingError{Code: "recovery_failed"}
	}
	saved, err = r.save(saved, next)
	if err != nil {
		return err
	}
	if !work.eligible {
		r.result.snapshot.Deferred++
		return r.issue(ProcessingIssue{NoticeID: work.id, Code: work.reason})
	}
	if reason := saved.Record().Snapshot().HoldReason; reason != "" {
		return r.issue(ProcessingIssue{NoticeID: work.id, Code: string(reason)})
	}
	saved, n, lookups, available, err := r.fresh(work, saved)
	if err != nil || !available {
		return err
	}
	saved, err = r.prepare(saved, n, lookups)
	if err != nil {
		return err
	}
	// IDs are immutable. Reload the current snapshot after every outcome so no
	// stale in-memory copy can overwrite another batch's confirmed result.
	for _, batch := range saved.Record().Snapshot().Batches {
		saved, err = r.attempt(work, saved, batch.Message.BatchID)
		if err != nil {
			return err
		}
	}
	return nil
}

func processingBatch(saved ocistorage.StoredNotice, id string) (delivery.BatchSnapshot, bool) {
	for _, b := range saved.Record().Snapshot().Batches {
		if b.Message.BatchID == id {
			return b, true
		}
	}
	return delivery.BatchSnapshot{}, false
}

func (r *processingRun) holdBatch(saved ocistorage.StoredNotice, b delivery.BatchSnapshot, reason delivery.Reason, code string) (ocistorage.StoredNotice, error) {
	next, err := saved.Record().HoldBatch(b.Message.BatchID, reason, r.lease.manager.now())
	if err != nil {
		return saved, &ProcessingError{Code: "transition_failed"}
	}
	got, err := r.save(saved, next)
	if err != nil {
		return saved, err
	}
	return got, r.issue(ProcessingIssue{NoticeID: b.Message.Revision.NoticeID, RevisionDigest: b.Message.Revision.Digest, BatchID: b.Message.BatchID, Code: code})
}
