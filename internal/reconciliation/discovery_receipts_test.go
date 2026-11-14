package reconciliation

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func discoveryReceipt(t *testing.T) (notices.Notice, noticeledger.Record, noticeledger.Record) {
	t.Helper()
	id := "ocid1.mysqldbsystem.oc1.iad.example"
	n := normalizeCandidate(t, notices.Record{NoticeID: noticeID(1), AnnouncementType: "SCHEDULED_MAINTENANCE", Platform: chain("IAAS"), Services: []string{"Oracle HeatWave MySQL"}, Resources: []notices.Resource{{ResourceID: id}}})
	lookups := map[string]routing.Lookup{id: {State: routing.Found, Metadata: routing.Metadata{ID: chain(id), CompartmentID: chain("ocid1.compartment.oc1..example"), FreeformTags: map[string]string{"owner": "payments"}}}}
	o, err := noticeledger.New([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := o.Observe(noticeledger.Record{}, n, false, lookups, at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := delivery.New([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := b.Build(n, false, lookups, []string{id})
	if err != nil || len(drafts.Drafts) != 1 {
		t.Fatal("draft", err)
	}
	r, err = r.Prepare(at, delivery.Preparation{Draft: drafts.Drafts[0], SecretVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	batch := r.Snapshot().Batches[0]
	r, ref, err := r.Begin(batch.Message.BatchID, delivery.AttemptContext{OwnerToken: strings.Repeat("a", 64), RoutingGeneration: batch.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := r.Finish(ref, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return n, r, accepted
}

func TestDiscoveryReloadPreservesNewReceiptAndStickyHold(t *testing.T) {
	n, sending, accepted := discoveryReceipt(t)
	accepted, err := accepted.Hold(delivery.ReasonOperator, time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, []noticeledger.Record{sending})
	reads := 0
	s.before = func(_ context.Context, call discoveryCall) error {
		if call.method == "GET" && call.name == recordName(sending) {
			reads++
			if reads == 2 {
				body, err := accepted.Encode()
				if err != nil {
					t.Fatal(err)
				}
				s.seed(call.name, body)
			}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	after := s.record(t, got.EnvironmentID(), n.Key().NoticeID).Snapshot()
	before := accepted.Snapshot()
	if !reflect.DeepEqual(after.Batches, before.Batches) || !reflect.DeepEqual(after.Revisions[0].Resources, before.Revisions[0].Resources) || after.HoldReason != delivery.ReasonOperator {
		t.Fatal("latest receipt, assignments or operator hold overwritten")
	}
	if after.SourceOutcome.State != noticeledger.SourceAvailable || len(s.noticeWrites()) != 1 {
		t.Fatal("source was not recorded once")
	}
}
