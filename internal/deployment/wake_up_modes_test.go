package deployment

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocimonitoring"
)

func TestAutomaticWakeUpDoesNothingOutsideLive(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "TEST", "PAUSED"} {
		for _, trigger := range []string{"schedule", "notification", "function-notification"} {
			t.Run(mode+"/"+trigger, func(t *testing.T) {
				h, s, objects, environment, principal := retainedTestFixture(t, true)
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = mode })
				s, _, _ = configureRecovery(t, h, s, objects, environment, noticeledger.CloseRevision)
				before := objects.version
				h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
					t.Fatal("automatic wake-up reached preparation/test/recovery")
					return operations{}, nil
				}
				h.metrics = func(Settings, Store, string) (metricPublisher, error) {
					t.Fatal("automatic nonLIVE wake-up constructed metrics")
					return nil, nil
				}
				headers, body := intakeHeaders(), invocation(s)
				if trigger == "schedule" {
					headers, body = nil, `{"operation":"wake-up"}`
				} else if trigger == "function-notification" {
					headers = functionNotificationHeaders()
				}
				got := automaticResponse(t, h, operationalContext(t), headers, strings.NewReader(body))
				if got.ResponseVersion != 2 || got.Status != "idle" || got.Operation != "reconcile" || got.Mode != mode || got.Code != "" || got.ScanCompleted || got.BaselinePrepared || got.BaselineSelected || got.Diagnostic != nil || got.Operator != nil || got.Recovery != nil || got.Processing != nil || got.Reporting != nil || got.Telemetry != nil || objects.version != before || principal.reads != 0 {
					t.Fatalf("automatic nonLIVE work or false success: %+v writes=%d/%d", got, objects.version, before)
				}
			})
		}
	}
}

func TestAutomaticLiveWakeUpUsesExistingFinalizedProcessing(t *testing.T) {
	for _, trigger := range []string{"schedule", "notification", "function-notification"} {
		h, _, objects, _ := liveOperationFixture(t)
		bindMetricRun(t, h)
		before, calls := objects.version, 0
		h.metrics = func(Settings, Store, string) (metricPublisher, error) {
			return observedMetrics{func(_ context.Context, o ocimonitoring.Observation) error {
				calls++
				if !o.CompletedScan || objects.version != before+2 {
					t.Error("automatic metric preceded cleanup or lost completion")
				}
				return nil
			}}, nil
		}
		headers, body := intakeHeaders(), `{"operation":"close-work","private":"ignored"}`
		if trigger == "schedule" {
			headers, body = nil, `{"operation":"wake-up"}`
		} else if trigger == "function-notification" {
			headers = functionNotificationHeaders()
		}
		got := automaticResponse(t, h, operationalContext(t), headers, strings.NewReader(body))
		if got.Status != "reconciled" || got.Operation != "reconcile" || !got.ScanCompleted || got.Processing == nil || got.Processing.Accepted != 2 || got.Telemetry == nil || !got.Telemetry.Accepted || calls != 1 {
			t.Fatalf("automatic work did not reuse processing: %+v", got)
		}
	}
}
