package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingAuthorityAndCurrentChannelFailuresCannotPublish(t *testing.T) {
	for _, kind := range []string{"missing-lease", "no-deadline", "token", "lease-token", "changed-current", "missing-receipt", "changed-channel", "channel-failure", "missing-publisher", "claim-conflict", "lost-current-after-claim"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			l := f.processing.live.lease
			ctx := leaseBudget(t)
			switch kind {
			case "missing-lease":
				l = nil
			case "no-deadline":
				ctx = context.Background()
			case "token":
				f.reporter.token = func() string { return "invalid" }
			case "lease-token":
				f.reporter.token = func() string { return l.record.OwnerToken }
			case "changed-current":
				f.processing.live.objects.seed("configuration/router.json", []byte("changed"))
			case "missing-receipt":
				f.processing.live.objects.seed(operatorRunObjectName, []byte("bad"))
			case "changed-channel", "channel-failure", "missing-publisher":
				read := f.reporter.read
				f.reporter.read = func(ctx context.Context) (operatorObservation, error) {
					c, e := read(ctx)
					if kind == "changed-channel" {
						c.channel.Endpoint = "different@example.invalid"
					}
					if kind == "channel-failure" {
						e = errors.New("private-channel-detail")
					}
					if kind == "missing-publisher" {
						c.publish = nil
					}
					return c, e
				}
			case "claim-conflict", "lost-current-after-claim":
				f.processing.live.objects.before = func(_ context.Context, c discoveryCall) error {
					if kind == "claim-conflict" && c.method == "REPLACE" && c.name == issueReportName && strings.Contains(string(c.body), `"claim":"`+strings.Repeat("d", 64)) {
						return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
					}
					return nil
				}
				f.processing.live.objects.after = func(_ context.Context, c discoveryCall) error {
					if c.method == "REPLACE" && c.name == issueReportName && strings.Contains(string(c.body), `"claim":"`+strings.Repeat("d", 64)) {
						f.processing.live.objects.seed("configuration/router.json", []byte("changed"))
					}
					return nil
				}
				f.reporter.token = func() string { return strings.Repeat("d", 64) }
			}
			got, err := f.reporter.Refresh(ctx, l, f.snapshot)
			if err == nil || got.Dispatched || f.sends != 0 || strings.Contains(err.Error(), "private-") {
				t.Fatal("invalid authority granted publication", got, err)
			}
		})
	}
}

func TestIssueReportingRejectsIncompleteProviderAcceptance(t *testing.T) {
	for _, kind := range []string{"wrong-status", "missing-id", "provider-error", "invalid-status"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			read := f.reporter.read
			f.reporter.read = func(ctx context.Context) (operatorObservation, error) {
				c, e := read(ctx)
				publish := c.publish
				c.publish = func(ctx context.Context, b []byte) (ocinotifications.Publication, error) {
					p, err := publish(ctx, b)
					switch kind {
					case "wrong-status":
						p.Status = 204
					case "invalid-status":
						p.Status = 600
					case "missing-id":
						p.MessageID = ""
					case "provider-error":
						err = errors.New("private-error")
					}
					return p, err
				}
				return c, e
			}
			got, err := f.reporter.Refresh(leaseBudget(t), f.processing.live.lease, f.snapshot)
			if err == nil || !got.Dispatched || got.Accepted || !got.Pending || f.saved(t).Accepted != 0 {
				t.Fatal("incomplete acceptance suppressed retry", got, err)
			}
		})
	}
}
