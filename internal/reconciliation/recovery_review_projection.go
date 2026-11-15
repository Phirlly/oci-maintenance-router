package reconciliation

import (
	"context"
	"reflect"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/routing"
)

func migrateReviewRecord(record noticeledger.Record, at time.Time) (noticeledger.Record, error) {
	var err error
	if record.Snapshot().SchemaVersion == 1 {
		record, err = record.MigrateSourceOutcome(at)
		if err != nil {
			return noticeledger.Record{}, err
		}
	}
	if record.Snapshot().SchemaVersion == 2 {
		return record.MigrateRecovery(at)
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
		get := r.mysql[target.Region]
		if get == nil {
			return nil, reviewFailure(ctx, "lookup_configuration")
		}
		metadata, err := get(ctx, target.ResourceID)
		if ctx.Err() != nil {
			return nil, reviewFailure(ctx, "canceled")
		}
		lookup := routing.Lookup{State: routing.Found, Metadata: metadata}
		if err != nil {
			lookup = routing.Lookup{State: routing.Unavailable}
			if e, ok := err.(*ocimysql.Error); ok && e != nil && e.Code == ocimysql.Service {
				if e.Status == 401 || e.Status == 403 {
					lookup.State = routing.Denied
				}
				if e.Status == 404 {
					lookup.State = routing.NotFound
				}
			}
		}
		lookups[target.ResourceID] = lookup
	}
	return lookups, nil
}
