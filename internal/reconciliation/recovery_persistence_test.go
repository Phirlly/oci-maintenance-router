package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestRecoveryMigrationAndClosureUseSeparateConfirmedParents(t *testing.T) {
	for _, lost := range []string{"none", "migration", "action"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.CloseRevision, n.Key().Digest)
		p := recoveryOnlyProcessor(t, f, "PAUSED", q)
		writes, previous := 0, ""
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if c.method != "REPLACE" || c.name != noticeName(n.Key().NoticeID) {
				return nil
			}
			writes++
			if c.tag == "" || c.tag == previous {
				t.Fatal("write reused migration predecessor")
			}
			previous = c.tag
			r, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, n.Key().NoticeID)
			if err != nil || r.Snapshot().SchemaVersion != 3 || len(r.Snapshot().Recovery.Actions) != writes-1 {
				t.Fatal("migration/action were not separate transitions", err)
			}
			if (lost == "migration" && writes == 1) || (lost == "action" && writes == 2) {
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		r, err := p.Run(leaseBudget(t), f.live.lease)
		if err != nil || !r.Snapshot().Completed || r.Snapshot().Recovery == nil || writes != 2 {
			t.Fatal("exact uncertain transition not confirmed", lost, err, writes)
		}
		f.live.objects.after = nil
	}
}

func TestRecoveryUnconfirmedTransitionsCannotClaimCompletion(t *testing.T) {
	for _, stage := range []string{"migration", "action"} {
		for _, fault := range []string{"conflict", "uncertain", "different-confirmation"} {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
			q := reviewedRecovery(t, old, noticeledger.CloseBatch, old.Snapshot().Batches[0].Message.BatchID)
			p := recoveryOnlyProcessor(t, f, "PAUSED", q)
			failed := false
			f.live.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method != "REPLACE" || c.name != noticeName(n.Key().NoticeID) {
					return nil
				}
				r, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, n.Key().NoticeID)
				if err != nil {
					t.Fatal(err)
				}
				if (len(r.Snapshot().Recovery.Actions) == 0) != (stage == "migration") {
					return nil
				}
				failed = true
				if fault == "conflict" {
					return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
				}
				if fault == "different-confirmation" {
					// A valid competing transition cannot confirm this proposal.
					other, err := old.Hold(delivery.ReasonOperator, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					body, _ := other.Encode()
					f.live.objects.seed(c.name, body)
				}
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			r, err := p.Run(leaseBudget(t), f.live.lease)
			if !failed || err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || f.sent.Load() != 0 {
				t.Fatal("unconfirmed transition reported completion", stage, fault, err)
			}
			f.live.objects.before = nil
		}
	}
}
