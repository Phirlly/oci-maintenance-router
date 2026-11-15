package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingConfirmsExactLostWritesWithoutRepublishing(t *testing.T) {
	f := issueReporterFixtureFor(t)
	f.processing.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == issueReportName && (c.method == "CREATE" || c.method == "REPLACE") {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot)
	if err != nil || !got.Accepted || f.sends != 1 || f.saved(t).Pending != nil {
		t.Fatal("exact lost response was not confirmed", got, err)
	}
}

func TestIssueReportingConflictNeverRebasesOrPublishes(t *testing.T) {
	f := issueReporterFixtureFor(t)
	f.processing.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.name == issueReportName && c.method == "CREATE" {
			return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
		}
		return nil
	}
	got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot)
	if err == nil || got.Dispatched || f.sends != 0 {
		t.Fatal("conflicting state granted publication", got, err)
	}
}

func TestIssueReportingCorruptCurrentObjectNeverReinitializes(t *testing.T) {
	f := issueReporterFixtureFor(t)
	bad := []byte(`{"schemaVersion":1}`)
	f.processing.live.objects.seed(issueReportName, bad)
	got, err := f.reporter.Retry(leaseBudget(t), f.processing.live.lease)
	if err == nil || got.Dispatched || f.sends != 0 {
		t.Fatal("corrupt current state was accepted", got, err)
	}
	object, _ := f.processing.live.objects.Get(leaseBudget(t), issueReportName)
	if !bytes.Equal(object.Body, bad) {
		t.Fatal("corrupt state overwritten")
	}
}
