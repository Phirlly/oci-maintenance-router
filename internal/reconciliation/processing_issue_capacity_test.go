package reconciliation

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
)

func TestProcessingPreparationIssueCapacityPreservesHistoryAndStopsApproval(t *testing.T) {
	ids := make([]string, 256)
	indexes := make([]int, 256)
	for i := range ids {
		ids[i] = fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.db%03d", i)
		indexes[i] = i
	}
	raw := processingNotice(t, 1, ids...).Snapshot()
	raw.Reference = chain("revision-8")
	current := normalizeCandidate(t, raw)
	f := processingFixtureFor(t, current)
	lookups := map[string]routing.Lookup{}
	for _, id := range ids {
		m, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), id)
		if err != nil {
			t.Fatal(err)
		}
		lookups[id] = routing.Lookup{State: routing.Found, Metadata: m}
	}
	var r noticeledger.Record
	var p noticeledger.PreparationIssues
	start := time.Now().Add(-time.Minute)
	for i := 0; i < 9; i++ {
		raw.Reference = chain(fmt.Sprintf("revision-%d", i))
		n := normalizeCandidate(t, raw)
		at := start.Add(time.Duration(i) * time.Second)
		var err error
		r, err = f.p.observer.Observe(r, n, true, lookups, at)
		if err != nil {
			t.Fatal(err)
		}
		if i == 8 {
			break
		}
		failure, err := f.p.observer.PreparationFailure(r, n, true, lookups, indexes, "secret_unavailable", at)
		if err != nil {
			t.Fatal(err)
		}
		p, err = p.Observe(failure)
		if err != nil {
			t.Fatal(err)
		}
	}
	body, _ := r.Encode()
	f.live.objects.seed(noticeName(current.Key().NoticeID), body)
	prior, _ := p.Encode()
	name := preparationIssueName(r)
	f.live.objects.seed(name, prior)
	q := reviewedRecovery(t, r, noticeledger.ApproveRevision, current.Key().Digest)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { return nil, nil }
	result, err := f.p.Run(leaseBudget(t), f.live.lease)
	if e, ok := err.(*ProcessingError); !ok || e.Code != "preparation_issue_capacity" || result.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("issue capacity allowed completion", err)
	}
	got, err := f.live.objects.Get(leaseBudget(t), name)
	if err != nil || !bytes.Equal(got.Body, prior) || preparationCalls(f.live.objects, "REPLACE", name) != 0 {
		t.Fatal("capacity failure discarded retained evidence", err)
	}
	if len(recoveryActions(t, f, current.Key().NoticeID)) != 0 {
		t.Fatal("capacity failure consumed approval")
	}
}
