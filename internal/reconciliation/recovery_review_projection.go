package reconciliation

import (
	"context"
	"reflect"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func migrateReviewRecord(record noticeledger.Record, at time.Time, announcement bool) (noticeledger.Record, error) {
	if !announcement && record.Snapshot().SchemaVersion == 4 {
		return noticeledger.Record{}, &RecoveryReviewError{Code: "unsupported_notice_schema"}
	}
	var err error
	if record.Snapshot().SchemaVersion == 1 {
		record, err = record.MigrateSourceOutcome(at)
		if err != nil {
			return noticeledger.Record{}, err
		}
	}
	if record.Snapshot().SchemaVersion == 2 {
		record, err = record.MigrateRecovery(at)
		if err != nil {
			return noticeledger.Record{}, err
		}
	}
	if announcement && record.Snapshot().SchemaVersion == 3 {
		return record.MigrateAnnouncements(at)
	}
	return record, nil
}

func projectReviewObservations(record noticeledger.Record, work *discoveryWork, at time.Time) (noticeledger.Record, error) {
	for _, observation := range work.observations {
		var err error
		record, err = observation.Apply(record, at)
		if err != nil {
			return noticeledger.Record{}, err
		}
	}
	if work.hold && record.Snapshot().HoldReason == "" || recoveryComponentChanged(record, work.component) {
		return record.Hold(delivery.ReasonSourceChanged, at)
	}
	return record, nil
}

func reviewNeedsRefresh(original, projected noticeledger.Record) bool {
	a, b := original.Snapshot(), projected.Snapshot()
	if !reflect.DeepEqual(a.Batches, b.Batches) {
		return true
	}
	if a.Recovery != nil {
		for i, action := range a.Recovery.Actions {
			if action.InvalidatedAt == nil && b.Recovery.Actions[i].InvalidatedAt != nil {
				return true
			}
		}
	}
	return false
}

func (r *RecoveryReviewer) currentLookups(ctx context.Context, n notices.Notice) (map[string]routing.Lookup, error) {
	_, targets := r.policy.Targets(n, true)
	lookups := map[string]routing.Lookup{}
	for _, target := range targets {
		if err := r.before(ctx); err != nil {
			return nil, err
		}
		get := r.reader(target)
		if get == nil {
			return nil, reviewFailure(ctx, "lookup_configuration")
		}
		metadata, err := get(ctx, target.ResourceID)
		if ctx.Err() != nil {
			return nil, reviewFailure(ctx, "canceled")
		}
		lookups[target.ResourceID] = ownershipResult(metadata, err)
	}
	return lookups, nil
}
