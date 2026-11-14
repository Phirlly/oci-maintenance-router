package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestRecoveryStartupFailureDoesNotDiscoverOrManufactureRejection(t *testing.T) {
	for _, fault := range []string{"current", "missing-notice", "invalid-notice"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.CloseRevision, n.Key().Digest)
		p := recoveryOnlyProcessor(t, f, "PAUSED", q)
		switch fault {
		case "current":
			f.live.objects.seed("configuration/router.json", []byte("changed"))
		case "invalid-notice":
			f.live.objects.seed(noticeName(n.Key().NoticeID), []byte("invalid"))
		case "missing-notice":
			f.live.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method == "GET" && c.name == noticeName(n.Key().NoticeID) {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 404}
				}
				return nil
			}
		}
		writes := len(f.live.objects.noticeWrites())
		r, err := p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || len(f.live.objects.noticeWrites()) != writes || f.listed.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("failed recovery startup changed work", fault, err)
		}
	}
}

func TestRecoveryReadinessRejectionRequiresConfirmedStorage(t *testing.T) {
	for _, approval := range []bool{false, true} {
		for _, atStart := range []bool{false, true} {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			old := seedRecoveryRevision(t, f, n)
			kind, target := noticeledger.ApproveRevision, n.Key().Digest
			if !approval {
				old = f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
				kind, target = noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID
			}
			q := reviewedRecovery(t, old, kind, target)
			f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
			invalidate := func() {
				f.p.selector.operator.read = func(context.Context) (OperatorChannel, error) {
					c := f.p.selector.operator.input.desired
					c.SubscriptionID = "changed"
					return c, nil
				}
			}
			if atStart {
				invalidate()
			}
			current, version := f.p.current, f.p.version
			f.p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
				k, e := current(ctx, id)
				invalidate()
				return k, e
			}
			f.p.version = func(ctx context.Context, id string, v int64) (maintenanceCredential, error) {
				k, e := version(ctx, id, v)
				invalidate()
				return k, e
			}
			blocked := false
			f.live.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method != "REPLACE" || c.name != noticeName(n.Key().NoticeID) {
					return nil
				}
				candidate, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, n.Key().NoticeID)
				if err != nil {
					t.Fatal(err)
				}
				if h := candidate.Snapshot().Recovery; h != nil && len(h.Actions) != 0 {
					blocked = true
					return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
				}
				return nil
			}
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			if !blocked || err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || f.sent.Load() != 0 || len(recoveryActions(t, f, n.Key().NoticeID)) != 0 {
				t.Fatal("unconfirmed rejection reported as retained", approval, atStart, err)
			}
		}
	}
}

func TestRecoveryApprovalWriteFailureNeverAuthorizesPreparedBatches(t *testing.T) {
	for _, lost := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := seedRecoveryRevision(t, f, n)
		q := reviewedRecovery(t, old, noticeledger.ApproveRevision, n.Key().Digest)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		blocked := false
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if c.method != "REPLACE" || c.name != noticeName(n.Key().NoticeID) {
				return nil
			}
			r, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, n.Key().NoticeID)
			if err != nil {
				t.Fatal(err)
			}
			if h := r.Snapshot().Recovery; h != nil && len(h.Actions) != 0 {
				blocked = true
				if lost {
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if !blocked || err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || r.Snapshot().Prepared != 0 || f.sent.Load() != 0 || len(recoveryActions(t, f, n.Key().NoticeID)) != 0 || len(f.saved(t, n.Key().NoticeID).Snapshot().Batches) != 0 {
			t.Fatal("failed approval write authorized partial work", lost, err)
		}
	}
}

func TestRecoveryReadinessRejectionCannotUseUnreadableTarget(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	missing := false
	f.p.selector.operator.read = func(context.Context) (OperatorChannel, error) {
		missing = true
		c := f.p.selector.operator.input.desired
		c.SubscriptionID = "changed"
		return c, nil
	}
	f.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if missing && c.method == "GET" && c.name == noticeName(n.Key().NoticeID) {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	f.live.objects.before = nil
	if !missing || err == nil || r.Snapshot().Recovery != nil || f.sent.Load() != 0 || len(recoveryActions(t, f, n.Key().NoticeID)) != 0 {
		t.Fatal("unreadable target manufactured rejection", err)
	}
}
