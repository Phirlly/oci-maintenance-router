package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const processingDB = "ocid1.mysqldbsystem.oc1.iad.database"

type processingFixture struct {
	live                                  *liveSelectorFixture
	p                                     *MaintenanceProcessor
	source                                announcementDetails
	listed, fetched, current, exact, sent atomic.Int32
	mu                                    sync.Mutex
	payloads                              [][]byte
}

func processingNotice(t *testing.T, number int, ids ...string) notices.Notice {
	t.Helper()
	r := notices.Record{NoticeID: noticeID(number), AnnouncementType: "SCHEDULED_MAINTENANCE", Services: []string{"Oracle HeatWave MySQL"}, Platform: chain("IAAS")}
	for _, id := range ids {
		r.Resources = append(r.Resources, notices.Resource{ResourceID: id})
	}
	return normalizeCandidate(t, r)
}

func processingFixtureFor(t *testing.T, ns ...notices.Notice) *processingFixture {
	t.Helper()
	return processingFixtureWithBaseline(t, nil, ns...)
}

func processingFixtureWithBaseline(t *testing.T, initial []notices.Notice, ns ...notices.Notice) *processingFixture {
	t.Helper()
	f := &processingFixture{live: liveSelectorFixtureFor(t, initial...)}
	ids, byID := []string{}, map[string]notices.Notice{}
	for _, n := range ns {
		ids = append(ids, n.Key().NoticeID)
		byID[n.Key().NoticeID] = n
	}
	f.source = announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) {
		f.listed.Add(1)
		return ociannouncements.Page{IDs: slices.Clone(ids)}, nil
	}, get: func(_ context.Context, id string) (notices.Notice, error) {
		f.fetched.Add(1)
		if n, ok := byID[id]; ok {
			return n, nil
		}
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}}
	var err error
	f.p, err = NewMaintenanceProcessor(f.live.config, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.source, f.live.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
	if err != nil {
		t.Fatal(err)
	}
	f.p.selector.operator.read = f.live.selector.operator.read
	f.p.discovery.wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	f.p.wait = f.p.discovery.wait
	f.p.jitter = func(time.Duration) time.Duration { return 0 }
	f.p.mysql["us-ashburn-1"] = func(_ context.Context, id string) (routing.Metadata, error) {
		return routing.Metadata{ID: chain(id), CompartmentID: chain("ocid1.compartment.oc1..example"), FreeformTags: map[string]string{"owner": "payments"}}, nil
	}
	material := diagnosticMaterial{7, func() []byte { return bytes.Repeat([]byte{'k'}, 32) }}
	f.p.current = func(context.Context, string) (maintenanceCredential, error) { f.current.Add(1); return material, nil }
	f.p.version = func(_ context.Context, _ string, version int64) (maintenanceCredential, error) {
		f.exact.Add(1)
		if version != 7 {
			t.Error("wrong version")
		}
		return material, nil
	}
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		f.sent.Add(1)
		if ctx.Err() != nil || !bytes.Equal(key, bytes.Repeat([]byte{'k'}, 32)) {
			t.Error("wrong send context/key")
		}
		found := false
		for _, id := range ids {
			for _, b := range f.saved(t, id).Snapshot().Batches {
				if bytes.Equal(body, b.Message.PayloadWithoutRoutingKey) {
					found = true
					if b.State != delivery.Sending || b.SecretVersion != 7 || b.OwnerToken == f.live.lease.record.OwnerToken {
						t.Error("send lacks distinct confirmed claim")
					}
				}
			}
		}
		if !found {
			t.Error("sent payload was not saved")
		}
		f.mu.Lock()
		f.payloads = append(f.payloads, bytes.Clone(body))
		f.mu.Unlock()
		return pagerduty.Response{Status: 202}, nil
	}
	return f
}

func (f *processingFixture) saved(t *testing.T, id string) noticeledger.Record {
	t.Helper()
	s, err := ocistorage.NewNoticeStore(f.live.objects, f.live.lease.manager.environmentID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Load(leaseBudget(t), id)
	if err != nil {
		t.Fatal(err)
	}
	return r.Record()
}

func (f *processingFixture) run(t *testing.T) ProcessingSnapshot {
	t.Helper()
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err != nil {
		t.Fatal("processing", err)
	}
	return r.Snapshot()
}
