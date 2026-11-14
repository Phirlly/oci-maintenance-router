package noticeledger

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestPreparationReviewRechecksChangedPublicationContextUnderSameRevision(t *testing.T) {
	for _, field := range []string{"created", "updated"} {
		t.Run(field, func(t *testing.T) {
			raw := source(db, otherDB)
			r := observed(t, raw, db, otherDB)
			p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "payload_too_large", clock(), 0))
			if err != nil {
				t.Fatal(err)
			}
			before, _ := p.Encode()
			if field == "created" {
				raw.TimeCreated = text("2026-01-02T00:00:00Z")
			} else {
				raw.TimeUpdated = text("2026-01-02T00:00:00Z")
			}
			n := normalized(t, raw)
			if n.Key().Digest != r.Snapshot().Revisions[0].Digest {
				t.Fatal("fixture changed content revision")
			}
			current, err := observer(t, nil).Observe(r, n, false, lookups(db, otherDB), clock().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("%x", sha256.Sum256(settings(nil)))
			view, err := current.Review(p, config)
			if err != nil || view.Preparation[0].Status != "RECHECK_REQUIRED" {
				t.Fatal("changed publication context retained LAST_BLOCKED", view, err)
			}
			failure, err := observer(t, nil).PreparationFailure(current, n, false, lookups(db, otherDB), []int{0}, "secret_unavailable", clock().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			next, err := p.Observe(failure)
			if err != nil {
				t.Fatal(err)
			}
			view, err = current.Review(next, config)
			if err != nil || view.Preparation[0].Status != "LAST_BLOCKED" || view.Preparation[0].Observation.FirstSeenAt != clock().Format(timeFormat) || view.Preparation[0].Observation.LastSeenAt != clock().Add(time.Second).Format(timeFormat) {
				t.Fatal("fresh evidence did not retain subject history", view, err)
			}
			after, _ := p.Encode()
			if !bytes.Equal(before, after) {
				t.Fatal("new context mutated prior evidence")
			}
		})
	}
}
