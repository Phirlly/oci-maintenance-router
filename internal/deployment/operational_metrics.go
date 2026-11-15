package deployment

import (
	"context"
	"errors"
	"time"

	"oci-maintenance-router/internal/ocimonitoring"
)

type metricPublisher interface {
	Publish(context.Context, ocimonitoring.Observation) error
}

func composeMetrics(s Settings, store Store, installation string) (metricPublisher, error) {
	managed, ok := store.(*managedStore)
	if !ok || managed.principal == nil {
		return nil, errors.New("managed metrics binding required")
	}
	return ocimonitoring.New(s.Region, s.TenancyID, s.MetricsCompartmentID, installation, managed.principal)
}

func metricObservation(result OperationalResult, at time.Time) ocimonitoring.Observation {
	o := ocimonitoring.Observation{At: at}
	if result.Mode != "LIVE" || result.Operation != "reconcile" || result.Status == "coalesced" {
		return o
	}
	report := result.Reporting
	o.CompletedScan = result.Status == "reconciled" && result.ScanCompleted && result.Code == "" && result.CleanupCode == "" && report != nil && report.Code == ""
	if report == nil {
		return o
	}
	if report.Known {
		count := report.Issues
		o.ReviewIssues = &count
	}
	if report.Code != "" || report.Known && !report.Pending {
		failed := report.Code != ""
		o.OperatorReportFailed = &failed
	}
	return o
}

func publishOperationalMetrics(original context.Context, publisher metricPublisher, result *OperationalResult) {
	observation := metricObservation(*result, time.Now())
	if !observation.CompletedScan && observation.ReviewIssues == nil && observation.OperatorReportFailed == nil {
		return
	}
	result.Telemetry = &TelemetryOutcome{}
	ctx, cancel := context.WithTimeout(original, 10*time.Second)
	defer cancel()
	var err error
	if ctx.Err() == nil {
		err = publisher.Publish(ctx, observation)
	}
	if err == nil && ctx.Err() == nil {
		result.Telemetry.Accepted = true
		return
	}
	result.Status = "not-ready"
	result.Telemetry.Code = "publication_unconfirmed"
	if ctx.Err() != nil {
		result.Telemetry.Code = "deadline_or_canceled"
	}
	if result.Code == "" {
		result.Code = "telemetry_failed"
	}
	// ScanCompleted describes completed work before telemetry. Do not discard it
	// or confirmed business outcomes merely because metric acknowledgement failed.
}
