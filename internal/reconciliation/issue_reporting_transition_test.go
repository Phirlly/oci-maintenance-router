package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingDoesNotPublishAnUnconfirmedRefresh(t *testing.T) {
	for _, kind := range []string{"priority", "projection", "pending", "proof-current", "history-current", "confirmation-current", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			o := f.processing.live.objects
			ctx := leaseBudget(t)
			if kind == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
				defer cancel()
			}
			o.before = func(_ context.Context, c discoveryCall) error {
				if c.name == issueReportName && c.method == "REPLACE" {
					body := string(c.body)
					if kind == "priority" && strings.Contains(body, `"priority":["`) {
						return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
					}
					if kind == "projection" && strings.Contains(body, `"kind":"PREPARATION"`) {
						return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
					}
					if kind == "pending" && strings.Contains(body, `"pending":`) {
						return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
					}
				}
				return nil
			}
			if kind == "proof-current" {
				read := f.reporter.proof.read
				f.reporter.proof.read = func(ctx context.Context) (OperatorChannel, error) {
					channel, e := read(ctx)
					o.seed("configuration/router.json", []byte("changed"))
					return channel, e
				}
			}
			if kind == "history-current" {
				o.before = func(_ context.Context, c discoveryCall) error {
					if c.method == "GET" && c.name == issueReportName {
						o.seed("configuration/router.json", []byte("changed"))
					}
					return nil
				}
			}
			if kind == "confirmation-current" {
				o.after = func(_ context.Context, c discoveryCall) error {
					if c.method == "CREATE" && c.name == issueReportName {
						o.seed("configuration/router.json", []byte("changed"))
						return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
					}
					return nil
				}
			}
			got, err := f.reporter.Refresh(ctx, f.processing.live.lease, f.snapshot)
			if err == nil || got.Dispatched || f.sends != 0 {
				t.Fatal("partial transition authorized report", got, err)
			}
		})
	}
}

func TestIssueReportingFailureCodesDoNotExposeForeignDetails(t *testing.T) {
	err := reportFailure(context.Background(), errors.New("private-storage-detail"))
	if err.Error() != "operator reporting: reporting_failed" {
		t.Fatal("provider detail exposed", err)
	}
}
