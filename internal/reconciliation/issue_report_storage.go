package reconciliation

import (
	"bytes"
	"context"
	"encoding/json"

	"oci-maintenance-router/internal/ocistorage"
)

func cloneReport(s reportState) reportState {
	body, _ := json.Marshal(s)
	var next reportState
	_ = json.Unmarshal(body, &next)
	return next
}

func (r *reportingRun) load() (storedReport, error) {
	ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
	defer cancel()
	object, err := r.owner.objects.Get(ctx, issueReportName)
	if err == nil && ctx.Err() == nil {
		state, failure := decodeReport(object.Body, r.owner.environment, r.owner.installation)
		if failure != nil || !baselineETag(object.ETag) {
			return storedReport{}, &ReportingError{"invalid_record"}
		}
		return storedReport{state: state, etag: object.ETag}, nil
	}
	e, typed := err.(*ocistorage.Error)
	if ctx.Err() != nil || !typed || e == nil || e.Code != ocistorage.Service || e.Status != 404 {
		return storedReport{}, &ReportingError{"read_failed"}
	}
	if err = r.guard(); err != nil {
		return storedReport{}, err
	}
	history, done := context.WithTimeout(r.ctx, discoveryReserve)
	defer done()
	empty, err := r.owner.objects.HistoryIsEmpty(history, issueReportName)
	if err != nil || history.Err() != nil || !empty {
		return storedReport{}, &ReportingError{"history_unavailable"}
	}
	return storedReport{state: newReport(r.owner.environment, r.owner.installation, r.owner.channel), absent: true}, nil
}

// save never rebases a conflict. An uncertain write can only be confirmed by an
// exact read of this proposal. Outcome-only writes retain the original deadline.
func (r *reportingRun) save(state reportState, outcome bool) error {
	body, err := encodeReport(state)
	if err != nil {
		return err
	}
	old, _ := encodeReport(r.saved.state)
	if !r.saved.absent && bytes.Equal(old, body) {
		return r.guard()
	}
	ctx, cancel := r.writeContext(outcome)
	defer cancel()
	if err = r.writeGuard(ctx, outcome); err != nil {
		return err
	}
	var tag string
	if r.saved.absent {
		tag, err = r.owner.objects.Create(ctx, issueReportName, body)
	} else {
		tag, err = r.owner.objects.Replace(ctx, issueReportName, r.saved.etag, body)
	}
	if err != nil || ctx.Err() != nil || !baselineETag(tag) {
		if e, ok := err.(*ocistorage.Error); ok && e != nil && e.Code == ocistorage.Conflict {
			return &ReportingError{"conflict"}
		}
		confirm, _ := baselineWriteDisposition(err)
		if err != nil && !confirm {
			return &ReportingError{"write_failed"}
		}
		if err = r.writeGuard(ctx, outcome); err != nil {
			return err
		}
		got, failure := r.owner.objects.Get(ctx, issueReportName)
		if failure != nil || ctx.Err() != nil || !baselineETag(got.ETag) || !bytes.Equal(body, got.Body) {
			return &ReportingError{"write_unconfirmed"}
		}
		tag = got.ETag
	}
	if err = r.writeGuard(ctx, outcome); err != nil {
		return err
	}
	r.saved = storedReport{state: state, etag: tag}
	return nil
}

func (r *reportingRun) writeContext(outcome bool) (context.Context, context.CancelFunc) {
	if outcome {
		return context.WithDeadline(context.WithoutCancel(r.ctx), r.deadline)
	}
	return context.WithTimeout(r.ctx, discoveryReserve)
}

func (r *reportingRun) writeGuard(ctx context.Context, outcome bool) error {
	if outcome {
		now := r.lease.manager.now()
		if ctx.Err() != nil || !leaseWallTime(now) || now.Before(r.observed) || !now.Before(r.deadline) {
			return &ReportingError{"outcome_unconfirmed"}
		}
		return nil
	}
	return r.guard()
}
