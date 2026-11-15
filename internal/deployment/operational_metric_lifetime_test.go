package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocimonitoring"
	"oci-maintenance-router/internal/reconciliation"
)

func TestMetricsPreserveCancellationAndOriginalProgress(t *testing.T) {
	for _, kind := range []string{"canceled", "expired", "late", "failure", "empty"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if kind == "canceled" {
				cancel()
			}
			if kind == "expired" {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
			}
			calls := 0
			publisher := observedMetrics{func(work context.Context, _ ocimonitoring.Observation) error {
				calls++
				if deadline, ok := work.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("lost metric bound")
				}
				if kind == "late" {
					cancel()
					return nil
				}
				return errors.New("private-provider-detail")
			}}
			result := OperationalResult{Mode: "LIVE", Operation: "reconcile", Status: "not-ready", Code: "original_failure", Processing: &ProcessingOutcome{Accepted: 2}, Reporting: &ReportingOutcome{Known: true}}
			if kind == "empty" {
				result.Reporting = nil
			}
			publishOperationalMetrics(ctx, publisher, &result)
			want := 0
			if kind == "late" || kind == "failure" {
				want = 1
			}
			if calls != want || result.Code != "original_failure" || result.Processing.Accepted != 2 || (result.Telemetry == nil) != (kind == "empty") {
				t.Fatalf("unsafe late work or lost progress %+v calls=%d", result, calls)
			}
			if result.Telemetry != nil && result.Telemetry.Accepted {
				t.Fatal("unconfirmed metric accepted")
			}
		})
	}
}

func TestLiveWorkReservesCleanupAndMetricPublicationSeparately(t *testing.T) {
	h, s, objects, _ := liveOperationFixture(t)
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	deadline := time.Now().Add(2 * time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	processed, cleanup := false, false
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		ops := successfulReporting()
		ops.process = func(work context.Context, _ *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
			processed = true
			got, _ := work.Deadline()
			// 10s metrics + 10s cleanup + existing 60s post-processing report.
			if !got.Equal(deadline.Add(-80 * time.Second)) {
				t.Fatalf("incorrect processing reserve: %s", got)
			}
			return reconciliation.ProcessingSnapshot{Completed: true}, nil
		}
		return ops, nil
	}
	objects.before = func(work context.Context, method, _ string) error {
		if processed && method == "REPLACE" {
			cleanup = true
			got, _ := work.Deadline()
			if got.After(deadline.Add(-10*time.Second)) || time.Until(got) > 10*time.Second {
				t.Fatal("cleanup consumed metric reserve")
			}
		}
		return nil
	}
	got := h.runOperational(ctx, time.Now(), s, objects, p, command{operation: "reconcile"})
	if got.Status != "reconciled" || !processed || !cleanup {
		t.Fatalf("reserved work failed %+v", got)
	}
}
