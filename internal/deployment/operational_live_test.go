package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestLiveDeploymentVerificationSelectsWithoutProcessing(t *testing.T) {
	h, s, _, baseline := liveOperationFixture(t)
	selected := 0
	h.compose = func(_ Settings, _ Store, _ pinnedInputs, operation string) (operations, error) {
		if operation != "deployment-check" {
			t.Fatal("unexpected command")
		}
		return operations{selectLive: func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.Baseline, error) {
			selected++
			if err := lease.Check(ctx, time.Second); err != nil {
				t.Fatal("selection lacks genuine ownership", err)
			}
			return baseline, nil // Synthetic selector seam; no managed receipt claim.
		}, process: func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
			t.Fatal("Apply processed business notifications")
			return reconciliation.ProcessingSnapshot{}, nil
		}}, nil
	}
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "operational-verified" || !got.BaselineSelected || got.BaselinePrepared || got.ScanCompleted || got.Processing != nil || selected != 1 {
		t.Fatalf("incorrect LIVE verification: %+v", got)
	}
}

func TestLiveReconcileReportsCompletionAndPreservesPartialFailure(t *testing.T) {
	for _, kind := range []string{"complete", "partial", "incomplete", "late-current"} {
		t.Run(kind, func(t *testing.T) {
			h, _, objects, _ := liveOperationFixture(t)
			called := 0
			h.compose = func(_ Settings, _ Store, _ pinnedInputs, operation string) (operations, error) {
				if operation != "reconcile" {
					t.Fatal("unexpected command")
				}
				ops := successfulReporting()
				ops.process = func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					called++
					if err := lease.Check(ctx, time.Second); err != nil {
						t.Fatal("processing lacks ownership", err)
					}
					// Synthetic processor boundary tests reporting, not real dispatch.
					result := reconciliation.ProcessingSnapshot{Completed: kind != "incomplete", Prepared: 2, Accepted: 1, Deferred: 1, Suppressed: 3,
						Issues:   []reconciliation.ProcessingIssue{{NoticeID: "private-source-id", Code: "review"}},
						Attempts: []reconciliation.ProcessingAttempt{{Status: 202, MayHaveSent: true, Confirmed: true}},
						Recovery: &reconciliation.ProcessingRecovery{RequestID: "recovery-1", Status: "REJECTED", Reason: "readiness_required"}}
					if kind == "late-current" {
						objects.pinned.catalog = []byte("changed")
					}
					if kind == "partial" {
						return result, errors.New("private-provider-error")
					}
					return result, nil
				}
				return ops, nil
			}
			got := invokeOperational(t, h, `{"operation":"reconcile"}`)
			if (got.Status == "reconciled") != (kind == "complete") || got.ScanCompleted != (kind == "complete") || called != 1 || got.Processing == nil || *got.Processing != (ProcessingOutcome{Prepared: 2, Accepted: 1, Deferred: 1, Suppressed: 3, Issues: 1, Attempts: 1}) || got.Recovery == nil || got.Recovery.Status != "REJECTED" {
				t.Fatalf("incorrect full/partial report: %+v", got)
			}
			if kind == "partial" && got.Code != "operation_failed" || kind == "late-current" && got.Code != "configuration_changed" || kind == "incomplete" && got.Code != "processing_incomplete" {
				t.Fatalf("unsafe failure code: %s", got.Code)
			}
		})
	}
}
