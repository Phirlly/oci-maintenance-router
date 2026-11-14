package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
	"time"
)

func TestMaintenanceLateOrInvalidOutcomeClockCannotReportAcceptance(t *testing.T) {
	for _, kind := range []string{"deadline", "rollback", "jitter"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.p.senders["US"] = func(c context.Context, _, _ []byte) (pagerduty.Response, error) {
			f.sent.Add(1)
			switch kind {
			case "deadline":
				deadline, _ := c.Deadline()
				f.live.lease.manager.now = func() time.Time { return deadline.Add(discoveryReserve + time.Millisecond) }
			case "rollback":
				f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
			case "jitter":
				return pagerduty.Response{Status: 503}, nil
			}
			return pagerduty.Response{Status: 202}, nil
		}
		if kind == "jitter" {
			f.p.jitter = func(time.Duration) time.Duration { return -1 }
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		s := r.Snapshot()
		if err == nil || s.Completed || s.Accepted != 0 || len(s.Attempts) != 1 || s.Attempts[0].Confirmed || f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Sending {
			t.Fatal("invalid outcome claimed success or erased pending attempt", kind)
		}
	}
}
