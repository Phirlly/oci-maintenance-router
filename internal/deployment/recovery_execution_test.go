package deployment

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocimonitoring"
	"oci-maintenance-router/internal/reconciliation"
)

func sendingRecoveryIntent(kind string) map[string]any {
	return map[string]any{"requestId": "r1-reviewed", "noticeId": "ocid1.announcement.oc1..reviewed", "kind": kind, "targetId": strings.Repeat("a", 64), "reviewFingerprint": strings.Repeat("b", 64), "routingGeneration": strings.Repeat("c", 64), "componentFingerprint": strings.Repeat("d", 64)}
}

func TestExplicitRecoveryReusesLiveFinalizationAndRecordedOutcomes(t *testing.T) {
	for _, kind := range []string{"RETRY_BATCH", "APPROVE_REVISION"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := liveOperationFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["recovery"] = sendingRecoveryIntent(kind) })
			status := "API_ACCEPTED"
			if kind == "APPROVE_REVISION" {
				status = "APPROVED"
			}
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				ops := successfulReporting()
				ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
					return reconciliation.ProcessingSnapshot{Completed: true, Recovery: &reconciliation.ProcessingRecovery{RequestID: "r1-reviewed", Status: status, Ambiguous: kind == "RETRY_BATCH"}}, nil
				}
				return ops, nil
			}
			before, published := objects.version, 0
			h.metrics = func(Settings, Store, string) (metricPublisher, error) {
				return observedMetrics{func(_ context.Context, o ocimonitoring.Observation) error {
					if !o.CompletedScan || objects.version != before+2 {
						t.Error("metric lost completion or preceded cleanup")
					}
					published++
					return nil
				}}, nil
			}
			got := invokeOperational(t, h, recoverInvocation(s, "r1-reviewed"))
			if got.Status != "recovery-verified" || got.Operation != "recover-work" || got.CheckAttemptID != testAttemptID || got.Recovery == nil || got.Recovery.Status != status || !got.ScanCompleted || got.Telemetry == nil || !got.Telemetry.Accepted || published != 1 {
				t.Fatal("wrong recovery result", got)
			}
		})
	}
}
