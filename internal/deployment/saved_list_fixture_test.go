package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

// Supplies service-page observations; all record reads and leases use the real
// stores and domain codecs over the existing synthetic object service.
type savedListObjects struct {
	*operationalStore
	pages  map[string]ocistorage.NoticePage
	starts []string
}

func (s *savedListObjects) ListNoticePage(ctx context.Context, start string) (ocistorage.NoticePage, error) {
	s.starts = append(s.starts, start)
	if err := s.begin(ctx, "LIST", "notices/"); err != nil {
		return ocistorage.NoticePage{}, err
	}
	return s.pages[start], nil
}

func newSavedListFixture(t *testing.T, count int) (savedReviewFixture, *savedListObjects, []string, map[string]string) {
	t.Helper()
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
	for i := 1; i < count; i++ {
		addListedNotice(t, f, fmt.Sprintf("ocid1.announcement.oc1..example%d", i))
	}
	if count == 0 {
		for name := range f.objects.objects {
			delete(f.objects.objects, name)
		}
	}
	names, ids := []string{}, map[string]string{}
	for name, obj := range f.objects.objects {
		var record struct{ NoticeID string }
		if json.Unmarshal(obj.Body, &record) != nil {
			t.Fatal("invalid fixture")
		}
		names, ids[name] = append(names, name), record.NoticeID
	}
	slices.Sort(names)
	objects := &savedListObjects{operationalStore: f.objects, pages: map[string]ocistorage.NoticePage{"": {Names: names}}}
	f.h.connect = func(Settings) (Store, error) { return objects, nil }
	return f, objects, names, ids
}

func addListedNotice(t *testing.T, f savedReviewFixture, id string) {
	t.Helper()
	raw := f.notice.Snapshot()
	raw.NoticeID = id
	n, err := notices.Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.observer.Observe(noticeledger.Record{}, n, false, f.lookups, reviewTime())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Create(operationalContext(t), r); err != nil {
		t.Fatal(err)
	}
}

func invokeSavedList(t *testing.T, h *Handler, cursor string) OperationalResult {
	t.Helper()
	out := httptest.NewRecorder()
	h.Serve(operationalContext(t), strings.NewReader(listInvocation(cursor)), out)
	var got OperationalResult
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil || out.Body.Len() > 8192 || got.ResponseVersion != 2 {
		t.Fatalf("invalid listing response: %v %s", err, out.Body)
	}
	if got.ScanCompleted || got.BaselinePrepared || got.BaselineSelected || got.Processing != nil || got.Recovery != nil || got.Diagnostic != nil || got.Operator != nil || got.Review != nil {
		t.Fatalf("listing performed other work: %+v", got)
	}
	for _, private := range []string{"private-", "vaultsecret", "operator@example.invalid", "receiptCode", "payload", "ownerToken", "notices/"} {
		if strings.Contains(out.Body.String(), private) {
			t.Fatalf("private field in listing: %s", private)
		}
	}
	if got.SavedNotices != nil {
		body, _ := json.Marshal(got.SavedNotices)
		if got.SavedNotices.Evidence != "RETAINED_IDS" || got.SavedNotices.NoticeIDs == nil || len(got.SavedNotices.NoticeIDs) > 10 || len(body) > 6500 {
			t.Fatalf("invalid listing payload: %s", body)
		}
	}
	return got
}
