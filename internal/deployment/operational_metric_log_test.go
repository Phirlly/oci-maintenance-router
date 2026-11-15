package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocimonitoring"
)

func TestFinalLogIncludesTelemetryFailureWithoutLosingCompletedWork(t *testing.T) {
	h, _, _, _ := liveOperationFixture(t)
	bindMetricRun(t, h)
	var logs bytes.Buffer
	h.logger = log.New(&logs, "", 0)
	h.metrics = func(Settings, Store, string) (metricPublisher, error) {
		return observedMetrics{func(context.Context, ocimonitoring.Observation) error {
			return errors.New("private-provider-message")
		}}, nil
	}
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(`{"operation":"reconcile"}`), out)
	var result struct {
		Status        string
		ScanCompleted bool
		Processing    *ProcessingOutcome
		Telemetry     *TelemetryOutcome
	}
	if json.Unmarshal(logs.Bytes(), &result) != nil || out.Code != 400 || result.Status != "not-ready" || !result.ScanCompleted || result.Processing == nil || result.Processing.Accepted != 2 || result.Telemetry == nil || result.Telemetry.Accepted || result.Telemetry.Code != "publication_unconfirmed" {
		t.Fatalf("premature log or lost outcome: %s", logs.String())
	}
	if bytes.Count(logs.Bytes(), []byte("\n")) != 1 || strings.Contains(logs.String()+out.Body.String(), "private-provider-message") {
		t.Fatal("duplicate or unsafe final telemetry output")
	}
}
