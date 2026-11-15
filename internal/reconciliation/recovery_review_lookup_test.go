package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/routing"
)

func TestRecoveryReviewShowsUnresolvedOwnershipWithoutAuthorizing(t *testing.T) {
	for _, kind := range []string{"unavailable", "denied", "not-found", "untagged", "missing-client", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			seedReviewRevision(t, f, n)
			r := reviewFixture(t, f)
			ctx, cancel := context.WithCancel(leaseBudget(t))
			defer cancel()
			deadline, _ := ctx.Deadline()
			calls := 0
			r.mysql = map[string]func(context.Context, string) (routing.Metadata, error){"us-ashburn-1": func(work context.Context, id string) (routing.Metadata, error) {
				calls++
				end, ok := work.Deadline()
				if !ok || end.After(deadline) || time.Until(end) > 120*time.Second || id != processingDB {
					t.Fatal("lookup lost review budget or selected target")
				}
				switch kind {
				case "denied":
					return routing.Metadata{}, &ocimysql.Error{Code: ocimysql.Service, Status: 403}
				case "not-found":
					return routing.Metadata{}, &ocimysql.Error{Code: ocimysql.Service, Status: 404}
				case "untagged":
					return routing.Metadata{ID: chain(id), CompartmentID: chain("ocid1.compartment.oc1..example")}, nil
				case "canceled":
					cancel()
					return routing.Metadata{}, nil
				}
				return routing.Metadata{}, errors.New("private")
			}}
			if kind == "missing-client" {
				delete(r.mysql, "us-ashburn-1")
			}
			view, err := r.Review(ctx, n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
			if kind == "missing-client" || kind == "canceled" {
				if err == nil || view.Saved.Snapshot().NoticeID != "" {
					t.Fatal("failed lookup returned evidence", err)
				}
				return
			}
			if err != nil || calls != 1 || view.Fingerprint != "" || view.Blocker != "no_eligible_work" || len(view.Routing.Decisions) != 1 || view.Routing.Decisions[0].Disposition == routing.Ready {
				t.Fatal("unresolved owner authorized", kind, view.Blocker, err)
			}
		})
	}
}
