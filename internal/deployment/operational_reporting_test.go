package deployment

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func successfulReporting() operations {
	return operations{
		reportRetry: func(context.Context, *reconciliation.Lease) (reconciliation.ReportingResult, error) {
			return reconciliation.ReportingResult{Known: true}, nil
		},
		reportRefresh: func(context.Context, *reconciliation.Lease, reconciliation.ProcessingSnapshot) (reconciliation.ReportingResult, error) {
			return reconciliation.ReportingResult{Known: true}, nil
		},
	}
}

func TestLiveReportingRejectsMissingBindingAndDeadline(t *testing.T) {
	got := OperationalResult{}
	if err := (operations{}).processLive(operationalContext(t), nil, &got); err == nil || got.Reporting != nil || got.Processing != nil {
		t.Fatal("missing coordinator admitted")
	}
	ops := successfulReporting()
	ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
		t.Fatal("unbounded processing started")
		return reconciliation.ProcessingSnapshot{}, nil
	}
	if err := ops.processLive(context.Background(), nil, &got); err == nil {
		t.Fatal("unbounded context admitted")
	}
}

func TestLiveReportingPreservesProcessingAndOriginalBudget(t *testing.T) {
	for _, failure := range []string{"none", "retry", "processing", "refresh", "both"} {
		t.Run(failure, func(t *testing.T) {
			ctx := operationalContext(t)
			deadline, _ := ctx.Deadline()
			calls := []string{}
			snapshot := reconciliation.ProcessingSnapshot{Completed: true, Accepted: 2,
				Issues: []reconciliation.ProcessingIssue{{NoticeID: "private-notice", Code: "review"}}}
			ops := operations{
				reportRetry: func(work context.Context, _ *reconciliation.Lease) (reconciliation.ReportingResult, error) {
					calls = append(calls, "retry")
					end, _ := work.Deadline()
					if end.After(deadline) || time.Until(end) > 30*time.Second || work.Err() != nil {
						t.Fatal("retry budget extended")
					}
					var err error
					if failure == "retry" {
						err = &reconciliation.ReportingError{Code: "publication_unconfirmed"}
					}
					return reconciliation.ReportingResult{Known: true, Dispatched: true, Accepted: err == nil}, err
				},
				process: func(work context.Context, _ *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					calls = append(calls, "process")
					end, _ := work.Deadline()
					if !end.Equal(deadline.Add(-60*time.Second)) || work.Err() != nil {
						t.Fatal("reporting reserve lost")
					}
					if failure == "processing" || failure == "both" {
						return snapshot, errors.New("private provider detail")
					}
					return snapshot, nil
				},
				reportRefresh: func(work context.Context, _ *reconciliation.Lease, got reconciliation.ProcessingSnapshot) (reconciliation.ReportingResult, error) {
					calls = append(calls, "refresh")
					end, _ := work.Deadline()
					if !end.Equal(deadline) || work.Err() != nil || !reflect.DeepEqual(got, snapshot) {
						t.Fatal("post-report lost original scope or partial processing")
					}
					var err error
					if failure == "refresh" || failure == "both" {
						err = &reconciliation.ReportingError{Code: "read_failed"}
					}
					return reconciliation.ReportingResult{Known: err == nil, Issues: 1, Reviewed: 2, Pending: true, Remaining: 3, Deferred: 4}, err
				},
			}
			got := OperationalResult{}
			err := ops.processLive(ctx, nil, &got)
			if !reflect.DeepEqual(calls, []string{"retry", "process", "refresh"}) || (err == nil) != (failure == "none") || got.Processing == nil || got.Processing.Accepted != 2 || got.Reporting == nil || !got.Reporting.Dispatched || got.Reporting.Accepted != (failure != "retry") {
				t.Fatal("reporting concealed progress or failure", got, calls, err)
			}
			if failure == "none" && (!got.Reporting.Known || got.Reporting.Issues != 1 || got.Reporting.Deferred != 4 || got.Reporting.Code != "") {
				t.Fatal("latest report lost", got.Reporting)
			}
			if failure == "both" && (operationCode(ctx, err) != "operation_failed" || got.Reporting.Code != "read_failed") {
				t.Fatal("independent failures not preserved", got.Reporting, err)
			}
		})
	}
}
