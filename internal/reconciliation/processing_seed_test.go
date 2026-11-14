package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
	"time"
)

func (f *processingFixture) configure(t *testing.T, change func(map[string]any)) {
	t.Helper()
	f.live.configure(t, change)
	old := f.p
	p, err := NewMaintenanceProcessor(f.live.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.source, f.live.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err != nil {
		t.Fatal(err)
	}
	p.selector.operator.read = f.live.selector.operator.read
	p.discovery.wait, p.wait, p.jitter = old.discovery.wait, old.wait, old.jitter
	p.current, p.version, p.senders, p.mysql = old.current, old.version, old.senders, old.mysql
	f.p = p
}

func (f *processingFixture) seedBatch(t *testing.T, n notices.Notice, at time.Time, state delivery.State, held bool) noticeledger.Record {
	t.Helper()
	lookups := map[string]routing.Lookup{}
	_, targets := f.p.policy.Targets(n, true)
	ids := []string{}
	for _, target := range targets {
		m, err := f.p.mysql[target.Region](leaseBudget(t), target.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		lookups[target.ResourceID] = routing.Lookup{State: routing.Found, Metadata: m}
		ids = append(ids, target.ResourceID)
	}
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, lookups, at)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := f.p.builder.Build(n, true, lookups, ids)
	if err != nil || len(drafts.Drafts) == 0 {
		t.Fatal("fixture drafts", err)
	}
	for _, d := range drafts.Drafts {
		r, err = r.Prepare(at, delivery.Preparation{Draft: d, SecretVersion: 7})
		if err != nil {
			t.Fatal(err)
		}
	}
	if state != delivery.Prepared {
		for _, b := range r.Snapshot().Batches {
			var ref delivery.AttemptRef
			r, ref, err = r.Begin(b.Message.BatchID, delivery.AttemptContext{OwnerToken: "previous-worker", RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if state == delivery.RetryWait {
				r, err = r.Finish(ref, 503, at, at.Add(time.Second))
			}
			if state == delivery.APIAccepted {
				r, err = r.Finish(ref, 202, at, time.Time{})
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if held {
		r, err = r.Hold(delivery.ReasonSourceChanged, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	return r
}
