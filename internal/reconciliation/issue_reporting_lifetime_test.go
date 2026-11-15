package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

func TestIssueReportingIssuedOutcomeRetainsOriginalLifetime(t *testing.T) {
	for _, kind := range []string{"canceled", "changed-current", "clock-reversed", "clock-expired", "late-response", "outcome-conflict"} {
		t.Run(kind, func(t *testing.T) {
			f := issueReporterFixtureFor(t)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			deadline, _ := ctx.Deadline()
			read := f.reporter.read
			f.reporter.read = func(ctx context.Context) (operatorObservation, error) {
				c, e := read(ctx)
				publish := c.publish
				c.publish = func(work context.Context, b []byte) (ocinotifications.Publication, error) {
					p, err := publish(work, b)
					switch kind {
					case "canceled":
						cancel()
					case "changed-current":
						f.processing.live.objects.seed("configuration/router.json", []byte("changed"))
					case "outcome-conflict":
						f.processing.live.objects.before = func(_ context.Context, c discoveryCall) error {
							if c.method == "REPLACE" && c.name == issueReportName {
								return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
							}
							return nil
						}
					default:
						now := time.Now()
						calls := 0
						f.processing.live.lease.manager.now = func() time.Time {
							calls++
							if kind == "late-response" || kind == "clock-expired" && calls > 1 {
								return deadline.Add(time.Second)
							}
							if kind == "clock-reversed" && calls > 1 {
								return now.Add(-time.Second)
							}
							return now
						}
					}
					return p, err
				}
				return c, e
			}
			got, err := f.reporter.Refresh(ctx, f.processing.live.lease, f.snapshot)
			want := kind == "canceled" || kind == "changed-current"
			if got.Accepted != want || got.Pending == want || f.sends != 1 || (err == nil) != want {
				t.Fatal("outcome lost original lifetime or revived work", got, err)
			}
		})
	}
}
