package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocimonitoring"
	"oci-maintenance-router/internal/reconciliation"
)

type observedMetrics struct {
	publish func(context.Context, ocimonitoring.Observation) error
}

func (m observedMetrics) Publish(ctx context.Context, o ocimonitoring.Observation) error {
	return m.publish(ctx, o)
}

func bindMetricRun(t *testing.T, h *Handler) {
	t.Helper()
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		ops := successfulReporting()
		ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
			return reconciliation.ProcessingSnapshot{Completed: true, Accepted: 2}, nil
		}
		return ops, nil
	}
}

func TestLiveMetricsRequirePreflightBeforeAnyOwnedWork(t *testing.T) {
	h, _, objects, _ := liveOperationFixture(t)
	before := objects.version
	constructed := 0
	h.metrics = func(Settings, Store, string) (metricPublisher, error) {
		constructed++
		return nil, errors.New("private failure")
	}
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		t.Fatal("work before monitoring binding")
		return operations{}, nil
	}
	got := invokeOperational(t, h, `{"operation":"reconcile"}`)
	if got.Status != "not-ready" || got.Code != "monitoring_configuration" || constructed != 1 || objects.version != before {
		t.Fatalf("unsafe preflight %+v calls=%d", got, constructed)
	}
}

func TestLiveMetricPublicationFollowsFinalCurrentAndCleanup(t *testing.T) {
	for _, kind := range []string{"success", "current", "cleanup", "telemetry"} {
		t.Run(kind, func(t *testing.T) {
			h, _, objects, _ := liveOperationFixture(t)
			bindMetricRun(t, h)
			finished := false
			originalCompose := h.compose
			h.compose = func(s Settings, store Store, p pinnedInputs, op string) (operations, error) {
				ops, err := originalCompose(s, store, p, op)
				process := ops.process
				ops.process = func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					result, err := process(ctx, lease)
					finished = true
					if kind == "current" {
						objects.pinned.catalog = []byte("changed")
					}
					return result, err
				}
				return ops, err
			}
			cleanupSeen := false
			objects.before = func(_ context.Context, method, _ string) error {
				if finished && method == "REPLACE" {
					cleanupSeen = true
					if kind == "cleanup" {
						return errors.New("private cleanup")
					}
				}
				return nil
			}
			calls := 0
			h.metrics = func(Settings, Store, string) (metricPublisher, error) {
				return observedMetrics{func(ctx context.Context, value ocimonitoring.Observation) error {
					calls++
					if !cleanupSeen || value.CompletedScan != (kind == "success" || kind == "telemetry") || time.Since(value.At) > time.Second {
						t.Error("metric preceded finalization or invented completion")
					}
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
						t.Error("unbounded metric request")
					}
					if kind == "telemetry" {
						return errors.New("private metric detail")
					}
					return nil
				}}, nil
			}
			got := invokeOperational(t, h, `{"operation":"reconcile"}`)
			if calls != 1 || got.Processing == nil || got.Processing.Accepted != 2 || (got.Status == "reconciled") != (kind == "success") || got.ScanCompleted != (kind == "success" || kind == "telemetry") {
				t.Fatalf("lost progress or monitoring %+v calls=%d", got, calls)
			}
		})
	}
}
