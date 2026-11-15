package deployment

import (
	"testing"
	"time"
)

func TestMetricProjectionRequiresFinalizedLiveReconciliation(t *testing.T) {
	at := time.Now()
	for _, kind := range []string{"complete", "operation-error", "cleanup-error", "reporting-error", "incomplete", "coalesced", "apply", "preview", "paused", "test", "review"} {
		t.Run(kind, func(t *testing.T) {
			r := OperationalResult{Operation: "reconcile", Mode: "LIVE", Status: "reconciled", ScanCompleted: true, Reporting: &ReportingOutcome{Known: true, Issues: 4}}
			switch kind {
			case "operation-error":
				r.Code = "configuration_changed"
			case "cleanup-error":
				r.CleanupCode = "release_failed"
			case "reporting-error":
				r.Reporting.Code = "publication_unconfirmed"
			case "incomplete":
				r.ScanCompleted = false
			case "coalesced":
				r.Status = "coalesced"
			case "apply":
				r.Operation = "deployment-check"
			case "preview":
				r.Mode = "PREVIEW"
			case "paused":
				r.Mode = "PAUSED"
			case "test":
				r.Mode = "TEST"
			case "review":
				r.Operation = "review-notice"
			}
			got := metricObservation(r, at)
			emits := kind != "coalesced" && kind != "apply" && kind != "preview" && kind != "paused" && kind != "test" && kind != "review"
			if got.CompletedScan != (kind == "complete") || (got.ReviewIssues != nil) != emits || !got.At.Equal(at) {
				t.Fatalf("unsafe projection %+v", got)
			}
			if got.ReviewIssues != nil && *got.ReviewIssues != 4 {
				t.Fatal("saved issue count changed")
			}
		})
	}
}

func TestReportingMetricKeepsFailureThroughCooldownUntilPositiveRecovery(t *testing.T) {
	r := OperationalResult{Operation: "reconcile", Mode: "LIVE", Status: "not-ready"}
	for _, tc := range []struct {
		name      string
		reporting *ReportingOutcome
		known     bool
		failure   int
	}{
		{"unread", nil, false, -1},
		{"unknown", &ReportingOutcome{}, false, -1},
		{"failed", &ReportingOutcome{Known: true, Pending: true, Code: "publication_unconfirmed"}, true, 1},
		{"cooldown", &ReportingOutcome{Known: true, Pending: true}, true, -1},
		{"accepted-new-pending", &ReportingOutcome{Known: true, Pending: true, Accepted: true}, true, -1},
		{"accepted", &ReportingOutcome{Known: true, Accepted: true}, true, 0},
		{"empty", &ReportingOutcome{Known: true}, true, 0},
		{"read-error", &ReportingOutcome{Code: "read_failed"}, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.Reporting = tc.reporting
			got := metricObservation(r, time.Now())
			value := -1
			if got.OperatorReportFailed != nil {
				value = 0
				if *got.OperatorReportFailed {
					value = 1
				}
			}
			if got.CompletedScan || (got.ReviewIssues != nil) != tc.known || value != tc.failure {
				t.Fatalf("unverified recovery or lost failure: %+v", got)
			}
		})
	}
}
