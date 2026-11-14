package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProcessingPreparationIssueStopsAfterActualLeaseReplacement(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	name := preparationIssueName(f.savedOrObserved(t, n))
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == name && c.method == "HISTORY" {
			other := f.live.lease.record
			other.OwnerToken = strings.Repeat("e", 64)
			f.live.objects.seed(workerLeaseName, other.bytes())
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 || preparationCalls(f.live.objects, "CREATE", name) != 0 {
		t.Fatal("lost lease wrote preparation evidence", err)
	}
}

func TestProcessingPreparationIssueHistoryDispatchHonorsOriginalReserve(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	name := preparationIssueName(f.savedOrObserved(t, n))
	f.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.name == name && c.method == "GET" {
			end := f.live.lease.deadline
			f.live.lease.manager.now = func() time.Time { return end }
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 || preparationCalls(f.live.objects, "HISTORY", name) != 0 || preparationCalls(f.live.objects, "CREATE", name) != 0 {
		t.Fatal("exhausted original budget dispatched history/write", err)
	}
}

func TestProcessingPreparationIssueSameObservationDoesNotRewriteHistory(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	at := time.Now().Add(time.Second)
	f.live.lease.manager.now = func() time.Time { return at }
	f.p.discovery.now = f.live.lease.manager.now
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	f.run(t)
	p := savedPreparationIssues(t, f, n.Key().NoticeID)
	before, _ := p.Encode()
	f.run(t)
	after, _ := savedPreparationIssues(t, f, n.Key().NoticeID).Encode()
	name := preparationIssueName(f.saved(t, n.Key().NoticeID))
	if string(before) != string(after) || preparationCalls(f.live.objects, "CREATE", name) != 1 || preparationCalls(f.live.objects, "REPLACE", name) != 0 || f.sent.Load() != 0 {
		t.Fatal("same observation fabricated new history")
	}
}
