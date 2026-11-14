package noticeledger

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryRetryRetainsControlledPrerequisiteRejections(t *testing.T) {
	for _, kind := range []string{"prepared", "sending", "accepted", "unavailable", "changed_source", "changed_generation", "missing"} {
		r := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
		id := r.Snapshot().Batches[0].Message.BatchID
		want := "no_eligible_work"
		var err error
		switch kind {
		case "sending", "accepted":
			var ref delivery.AttemptRef
			r, ref, err = r.Begin(id, approvedContext(r, id, clock()))
			if err != nil {
				t.Fatal(err)
			}
			want = "active_attempt"
			if kind == "accepted" {
				r, err = r.Finish(ref, 202, clock(), time.Time{})
				if err != nil {
					t.Fatal(err)
				}
				want = "target_terminal"
			}
		case "unavailable", "changed_source", "changed_generation":
			r, err = r.Hold(delivery.ReasonOperator, clock())
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unavailable":
				r, err = r.RecordSourceUnavailable(404, clock())
				want = "source_unavailable"
			case "changed_source":
				n := source(db)
				n.Reference = text("new revision")
				r, err = observer(t, nil).Observe(r, normalized(t, n), true, lookups(db), clock())
				want = "source_changed"
			case "changed_generation":
				r, err = observer(t, func(v map[string]any) { v["pagerDutyRegion"] = "EU" }).Observe(r, normalized(t, source(db)), false, lookups(db), clock())
				want = "configuration_changed"
			}
			if err != nil {
				t.Fatal(err)
			}
		case "missing":
			want = "target_missing"
		}
		q := retryRequest(t, r)
		if kind == "changed_generation" {
			q.RoutingGeneration = r.Snapshot().Batches[0].Message.RoutingGeneration
			q.ReviewFingerprint, err = r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, q.ComponentFingerprint)
			if err != nil {
				t.Fatal(err)
			}
		}
		if kind == "missing" {
			q.TargetID = strings.Repeat("a", 64)
		}
		next, a, ref, err := r.RetryRecovery(q, retryContext(r, clock().Add(time.Second)))
		if err != nil || a.Status != RecoveryRejected || a.Reason != want || ref != (delivery.AttemptRef{}) || !reflect.DeepEqual(next.Snapshot().Batches, r.Snapshot().Batches) {
			t.Fatal(kind, a, err)
		}
		reload(t, next)
	}
}

func TestRecoveryRetryInvalidCallerMetadataNeverConsumesIneligibleRequest(t *testing.T) {
	r := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
	for _, missing := range []bool{false, true} {
		for _, kind := range []string{"owner", "generation", "expiry", "long_expiry", "zero_expiry"} {
			q := retryRequest(t, r)
			if missing {
				q.TargetID = strings.Repeat("a", 64)
			}
			c := retryContext(r, clock())
			switch kind {
			case "owner":
				c.OwnerToken = ""
			case "generation":
				c.RoutingGeneration = "bad"
			case "expiry":
				c.LeaseExpiresAt = c.Now
			case "long_expiry":
				c.LeaseExpiresAt = c.Now.Add(601 * time.Second)
			case "zero_expiry":
				c.LeaseExpiresAt = time.Time{}
			}
			bad, a, ref, err := r.RetryRecovery(q, c)
			rejected(t, bad, err)
			if a.Status != "" || ref != (delivery.AttemptRef{}) {
				t.Fatal("invalid caller consumed request", kind, missing)
			}
		}
	}
}
