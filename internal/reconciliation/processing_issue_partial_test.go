package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
)

func TestProcessingPreparationIssueSurvivesLaterSecretInterruption(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { cancel(); return nil, nil }
	r, err := f.p.Run(ctx, f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("interrupted pass completed", err)
	}
	p := savedPreparationIssues(t, f, n.Key().NoticeID).Snapshot()
	if len(p.Observations) != 1 || p.Observations[0].Code != "payload_too_large" {
		t.Fatal("earlier observation lost on later failure")
	}
}

func TestProcessingLaterIssueFailurePreservesAcceptedReceiptAndNeverReplays(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	f.run(t)
	before := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	at := time.Now().Add(time.Second)
	f.live.lease.manager.now = func() time.Time { return at }
	f.p.discovery.now = f.live.lease.manager.now
	f.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if strings.HasPrefix(c.name, "review/preparation/") && c.method == "REPLACE" {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed {
		t.Fatal("failed update reported complete", err)
	}
	after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if after.State != delivery.APIAccepted || after.Message.PayloadDigest != before.Message.PayloadDigest || after.AttemptCount != before.AttemptCount || f.sent.Load() != 1 {
		t.Fatal("issue failure changed accepted work")
	}
	f.live.objects.before = nil
	f.run(t)
	if f.sent.Load() != 1 {
		t.Fatal("accepted work replayed after recovery")
	}
}

func TestProcessingPreparationIssueValidOlderConfirmationIsNotRebased(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	f.run(t)
	old := savedPreparationIssues(t, f, n.Key().NoticeID)
	oldBytes, _ := old.Encode()
	at := time.Now().Add(time.Second)
	f.live.lease.manager.now = func() time.Time { return at }
	f.p.discovery.now = f.live.lease.manager.now
	name := preparationIssueName(f.saved(t, n.Key().NoticeID))
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == name && c.method == "REPLACE" {
			f.live.objects.seed(name, oldBytes)
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if e, ok := err.(*ProcessingError); !ok || e.Code != "issue_confirmation_mismatch" || !e.WriteUncertain || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("different valid state confirmed proposal", err)
	}
	if preparationCalls(f.live.objects, "REPLACE", name) != 1 {
		t.Fatal("mismatch rebased or retried")
	}
}
