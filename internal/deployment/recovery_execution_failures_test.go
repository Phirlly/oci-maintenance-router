package deployment

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocimonitoring"
	"oci-maintenance-router/internal/reconciliation"
)

func TestRecoveryPreservesConfirmedActionWhenLaterWorkFails(t *testing.T) {
	for _, kind := range []string{"processing", "reporting", "current", "cleanup", "telemetry"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := liveOperationFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["recovery"] = sendingRecoveryIntent("RETRY_BATCH") })
			finished := false
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				ops := successfulReporting()
				ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					finished = true
					r := reconciliation.ProcessingSnapshot{Completed: true, Accepted: 1, Recovery: &reconciliation.ProcessingRecovery{RequestID: "r1-reviewed", Status: "API_ACCEPTED", Ambiguous: true}}
					if kind == "current" {
						objects.pinned.catalog = []byte("changed")
					}
					if kind == "processing" {
						return r, errors.New("private")
					}
					return r, nil
				}
				if kind == "reporting" {
					ops.reportRefresh = func(context.Context, *reconciliation.Lease, reconciliation.ProcessingSnapshot) (reconciliation.ReportingResult, error) {
						return reconciliation.ReportingResult{}, errors.New("private")
					}
				}
				return ops, nil
			}
			objects.before = func(_ context.Context, method, name string) error {
				if finished && kind == "cleanup" && method == "REPLACE" && name == "control/lease.json" {
					return errors.New("private")
				}
				return nil
			}
			h.metrics = func(Settings, Store, string) (metricPublisher, error) {
				return observedMetrics{func(_ context.Context, o ocimonitoring.Observation) error {
					if o.CompletedScan != (kind == "telemetry") {
						t.Error("invented completion metric")
					}
					if kind == "telemetry" {
						return errors.New("private")
					}
					return nil
				}}, nil
			}
			got := invokeOperational(t, h, recoverInvocation(s, "r1-reviewed"))
			if got.Status != "not-ready" || got.Recovery == nil || got.Recovery.Status != "API_ACCEPTED" || !got.Recovery.Ambiguous || got.Processing == nil || got.Processing.Accepted != 1 || got.ScanCompleted != (kind == "telemetry") {
				t.Fatal("failure lost confirmation or claimed completion", got)
			}
		})
	}
}
