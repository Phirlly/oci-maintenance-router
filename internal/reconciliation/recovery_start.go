package reconciliation

import (
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func (r *processingRun) recoveryRequest() noticeledger.RecoveryRequest {
	q := r.owner.config.Recovery
	return noticeledger.RecoveryRequest{RequestID: q.RequestID, Kind: noticeledger.ActionKind(q.Kind),
		TargetID: q.TargetID, ReviewFingerprint: q.ReviewFingerprint,
		RoutingGeneration: q.RoutingGeneration, ComponentFingerprint: q.ComponentFingerprint}
}

// recoveryStart checks retained intent before discovery, expiry or credentials.
// A true result is a new sending request requiring the full LIVE preflight.
func (r *processingRun) recoveryStart() (bool, error) {
	q := r.owner.config.Recovery
	if q == nil {
		return false, nil
	}
	if err := r.guard(); err != nil {
		return false, err
	}
	saved, err := r.store.Load(r.ctx, q.NoticeID)
	if err != nil {
		return false, err
	}
	saved, err = r.migrateRecovery(saved)
	if err != nil {
		return false, err
	}
	_, exists, err := saved.Record().LookupRecovery(r.recoveryRequest())
	if err != nil {
		return false, &ProcessingError{Code: "recovery_intent_conflict"}
	}
	if exists {
		r.captureRecovery(saved)
		return false, r.guard()
	}
	generation, _ := r.owner.config.RoutingGeneration()
	if q.RoutingGeneration != generation {
		return false, r.rejectRecovery(saved, "configuration_changed")
	}
	if q.Kind == string(noticeledger.CloseBatch) || q.Kind == string(noticeledger.CloseRevision) {
		next, _, err := saved.Record().CloseRecovery(r.recoveryRequest(), r.lease.manager.now())
		if err != nil {
			return false, &ProcessingError{Code: "recovery_failed"}
		}
		_, err = r.save(saved, next)
		return false, err
	}
	if r.owner.config.Mode != "LIVE" {
		return false, r.rejectRecovery(saved, "readiness_required")
	}
	return true, nil
}

func (r *processingRun) migrateRecovery(saved ocistorage.StoredNotice) (ocistorage.StoredNotice, error) {
	target := 3
	if r.owner.config.SchemaVersion == 9 {
		target = 4
	}
	for saved.Record().Snapshot().SchemaVersion != target {
		var next noticeledger.Record
		var err error
		switch saved.Record().Snapshot().SchemaVersion {
		case 1:
			next, err = saved.Record().MigrateSourceOutcome(r.lease.manager.now())
		case 2:
			next, err = saved.Record().MigrateRecovery(r.lease.manager.now())
		case 3:
			next, err = saved.Record().MigrateAnnouncements(r.lease.manager.now())
		default:
			return saved, &ProcessingError{Code: "unsupported_notice_schema"}
		}
		if err != nil {
			return saved, &ProcessingError{Code: "recovery_migration_failed"}
		}
		saved, err = r.save(saved, next)
		if err != nil {
			return saved, err
		}
	}
	return saved, nil
}

func (r *processingRun) rejectRecovery(saved ocistorage.StoredNotice, reason string) error {
	next, _, err := saved.Record().RejectRecovery(r.recoveryRequest(), reason, r.lease.manager.now())
	if err != nil {
		return &ProcessingError{Code: "recovery_failed"}
	}
	_, err = r.save(saved, next)
	return err
}

func (r *processingRun) captureRecovery(saved ocistorage.StoredNotice) {
	q := r.owner.config.Recovery
	if q == nil || saved.Record().Snapshot().NoticeID != q.NoticeID {
		return
	}
	a, exists, err := saved.Record().LookupRecovery(r.recoveryRequest())
	if err != nil || !exists {
		return
	}
	r.result.snapshot.Recovery = &ProcessingRecovery{NoticeID: q.NoticeID,
		RequestID: a.Request.RequestID, Status: string(a.Status), Reason: a.Reason,
		InvalidationReason: a.InvalidationReason, Ambiguous: a.Ambiguous}
}
