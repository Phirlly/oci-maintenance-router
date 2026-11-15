package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingCannotResetUnknownOrDeletedHistory(t *testing.T) {
	for _, kind := range []string{"denied", "history-unavailable", "history-retained", "uncertain-write", "rejected-write", "changed-after-create"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			o := f.processing.live.objects
			o.before = func(_ context.Context, c discoveryCall) error {
				if c.name != issueReportName {
					return nil
				}
				if kind == "denied" && c.method == "GET" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				if kind == "history-unavailable" && c.method == "HISTORY" {
					return errors.New("private-history")
				}
				if c.method == "CREATE" {
					if kind == "uncertain-write" {
						return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
					}
					if kind == "rejected-write" {
						return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
					}
				}
				return nil
			}
			if kind == "history-retained" {
				o.history = func(context.Context, string) (bool, error) { return false, nil }
			}
			if kind == "changed-after-create" {
				o.after = func(_ context.Context, c discoveryCall) error {
					if c.method == "CREATE" && c.name == issueReportName {
						o.seed("configuration/router.json", []byte("changed"))
					}
					return nil
				}
			}
			got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot)
			if err == nil || got.Dispatched || f.sends != 0 || strings.Contains(err.Error(), "private-") {
				t.Fatal("unknown persistence authorized publication", got, err)
			}
		})
	}
}

func TestIssueReportingReadFailuresKeepPriorEvidenceAndBoundProgress(t *testing.T) {
	for _, kind := range []string{"sidecar", "listing", "invalid-hint", "cancel-notice", "cancel-sidecar", "changed-notice"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			o := f.processing.live.objects
			if _, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			o.before = func(_ context.Context, c discoveryCall) error {
				if kind == "listing" && c.method == "LIST" {
					return errors.New("private-list")
				}
				if c.method != "GET" {
					return nil
				}
				if strings.HasPrefix(c.name, "review/preparation/") {
					if kind == "sidecar" {
						return errors.New("private-sidecar")
					}
					if kind == "cancel-sidecar" {
						cancel()
					}
				}
				if strings.HasPrefix(c.name, "notices/") {
					if kind == "cancel-notice" {
						cancel()
					}
					if kind == "changed-notice" {
						o.seed("configuration/router.json", []byte("changed"))
					}
				}
				return nil
			}
			hint := ProcessingSnapshot{}
			if kind == "invalid-hint" {
				hint.Issues = []ProcessingIssue{{NoticeID: "private-invalid"}}
			}
			got, err := f.reporter.Refresh(ctx, f.processing.live.lease, hint)
			wantSuccess := kind == "sidecar"
			if (err == nil) != wantSuccess || got.Issues < 1 || f.sends != 1 {
				t.Fatal("read failure erased evidence or reset cooldown", got, err)
			}
			if wantSuccess {
				if got.Issues != 2 {
					t.Fatal("unreadable sidecar not visible", got)
				}
				revision := f.saved(t).Revision
				if repeated, e := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, ProcessingSnapshot{}); e != nil || repeated.Issues != 2 || f.saved(t).Revision != revision || f.sends != 1 {
					t.Fatal("repeated unreadable sidecar duplicated evidence or publication", repeated, e)
				}
				o.before = nil
				got, err = f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, ProcessingSnapshot{})
				if err != nil || got.Issues != 1 {
					t.Fatal("positive sidecar read did not resolve storage problem", got, err)
				}
			}
		})
	}
}
