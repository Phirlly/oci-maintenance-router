package deployment

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/reconciliation"
	"oci-maintenance-router/internal/routing"
)

type savedReviewFixture struct {
	h           *Handler
	s           Settings
	objects     *operationalStore
	environment string
	observer    *noticeledger.Observer
	notice      notices.Notice
	lookups     map[string]routing.Lookup
	record      noticeledger.Record
	store       *ocistorage.NoticeStore
	saved       ocistorage.StoredNotice
}

func reviewTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func newSavedReviewFixture(t *testing.T, id string, count int, ready bool) savedReviewFixture {
	t.Helper()
	h, s, objects, env := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	o, err := noticeledger.New(p.configBytes, p.catalog, configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket})
	if err != nil {
		t.Fatal(err)
	}
	platform, reference := "IAAS", "private-reference-not-for-output"
	raw := notices.Record{NoticeID: id, AnnouncementType: "SCHEDULED_MAINTENANCE", Services: []string{"Oracle HeatWave MySQL"}, Platform: &platform, Reference: &reference}
	lookups := map[string]routing.Lookup{}
	for i := 0; i < count; i++ {
		resource, compartment := fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.example%03d", i), "ocid1.compartment.oc1..example"
		raw.Resources = append(raw.Resources, notices.Resource{ResourceID: resource})
		if ready {
			lookups[resource] = routing.Lookup{State: routing.Found, Metadata: routing.Metadata{ID: &resource, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "payments"}}}}
		}
	}
	n, err := notices.Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, err := o.Observe(noticeledger.Record{}, n, false, lookups, reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	store, err := ocistorage.NewNoticeStore(objects, env)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(operationalContext(t), r)
	if err != nil {
		t.Fatal(err)
	}
	return savedReviewFixture{h, s, objects, env, o, n, lookups, r, store, saved}
}

func (f *savedReviewFixture) save(t *testing.T, next noticeledger.Record) {
	t.Helper()
	saved, err := f.store.Save(operationalContext(t), f.saved, next)
	if err != nil {
		t.Fatal(err)
	}
	f.saved = saved
	f.record = next
}

func (f *savedReviewFixture) preparationIssue(t *testing.T, index int) {
	t.Helper()
	failure, err := f.observer.PreparationFailure(f.record, f.notice, false, f.lookups, []int{index}, "secret_unavailable", reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	store, err := reconciliation.NewPreparationIssueStore(f.objects, f.environment)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(operationalContext(t), f.record)
	if err != nil {
		t.Fatal(err)
	}
	next, err := saved.Record().Observe(failure)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(operationalContext(t), saved, next); err != nil {
		t.Fatal(err)
	}
}

func invokeSavedReview(t *testing.T, h *Handler, id, cursor string) OperationalResult {
	t.Helper()
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(reviewInvocation(id, cursor)), out)
	var got OperationalResult
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil || out.Body.Len() > 8192 || got.ResponseVersion != 2 {
		t.Fatalf("invalid review response: %v %s", err, out.Body)
	}
	if got.ScanCompleted || got.BaselinePrepared || got.BaselineSelected || got.Processing != nil || got.Recovery != nil || got.Diagnostic != nil || got.Operator != nil {
		t.Fatalf("inspection performed other work: %+v", got)
	}
	for _, private := range []string{"private-reference-not-for-output", "vaultsecret", "operator@example.invalid", "receiptCode", "payload", "ownerToken"} {
		if strings.Contains(out.Body.String(), private) {
			t.Fatalf("private field in review: %s", private)
		}
	}
	return got
}
