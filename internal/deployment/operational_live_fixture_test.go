package deployment

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocimonitoring"
	"oci-maintenance-router/internal/reconciliation"
)

func liveOperationFixture(t *testing.T) (*Handler, Settings, *operationalStore, reconciliation.Baseline) {
	t.Helper()
	h, s, objects, environment, _ := retainedTestFixture(t, true)
	diagnosticCode := retainDiagnostic(t, s, objects, environment, 202)
	operatorCode := retainOperator(t, objects, environment, 200)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
	bindActualPreparation(t, h, &emptyAnnouncementSource{})
	if got := invokeOperational(t, h, invocation(s)); got.Status != "operational-verified" {
		t.Fatalf("baseline setup: %+v", got)
	}
	history, err := reconciliation.NewPreparedHistoryStore(objects, environment, "synthetic-installation")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := history.Load(operationalContext(t))
	if err != nil {
		t.Fatal(err)
	}
	baselines, err := reconciliation.NewBaselineStore(objects, environment)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := baselines.Load(operationalContext(t), saved.History().BaselineID())
	if err != nil {
		t.Fatal(err)
	}
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		v["mode"] = "LIVE"
		v["diagnostic"].(map[string]any)["receiptCode"] = diagnosticCode
		v["operatorTest"].(map[string]any)["receiptCode"] = operatorCode
	})
	h.compose = composeOperations
	// Existing LIVE tests isolate processing. Metric integration has dedicated
	// tests that override this seam; this fake makes no managed-ingestion claim.
	h.metrics = func(Settings, Store, string) (metricPublisher, error) {
		return observedMetrics{func(context.Context, ocimonitoring.Observation) error { return nil }}, nil
	}
	return h, s, objects, baseline
}
