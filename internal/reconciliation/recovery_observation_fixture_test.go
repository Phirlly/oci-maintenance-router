package reconciliation

import (
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"testing"
	"time"
)

func seedRecoveryRevision(t *testing.T, f *processingFixture, n notices.Notice) noticeledger.Record {
	t.Helper()
	lookups := map[string]routing.Lookup{}
	_, targets := f.p.policy.Targets(n, true)
	for _, target := range targets {
		m, err := f.p.mysql[target.Region](leaseBudget(t), target.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		lookups[target.ResourceID] = routing.Lookup{State: routing.Found, Metadata: m}
	}
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	return r
}
