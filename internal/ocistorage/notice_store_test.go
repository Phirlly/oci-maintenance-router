package ocistorage

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestNoticeStorePersistsCompleteRecordAndExactTransition(t *testing.T) {
	c, _ := conditionalServer(t, false)
	r := noticeState(t)
	s := noticeStore(t, c, r)
	created, e := s.Create(budget(t), r)
	if e != nil {
		t.Fatal(e)
	}
	if created.Record().Snapshot().NoticeID != stateNoticeID {
		t.Fatal("create returned no persisted record")
	}
	want, _ := r.Encode()
	key := fmt.Sprintf("notices/%x.json", sha256.Sum256([]byte(stateNoticeID)))
	raw, e := c.Get(budget(t), key)
	if e != nil || !bytes.Equal(want, raw.Body) {
		t.Fatal("whole record not persisted")
	}
	loaded, e := s.Load(budget(t), stateNoticeID)
	if e != nil {
		t.Fatal(e)
	}
	held := holdState(t, loaded.Record(), delivery.ReasonOperator)
	saved, e := s.Save(budget(t), loaded, held)
	if e != nil || saved.Record().Snapshot().HoldReason != delivery.ReasonOperator {
		t.Fatal("conditional save failed")
	}
	loaded, e = s.Load(budget(t), stateNoticeID)
	if e != nil || loaded.Record().Snapshot().Batches[0].State != delivery.ReviewRequired {
		t.Fatal("reload lost delivery disposition")
	}
}

func TestNoticeStoreRejectsUnboundOrStaleProposalsBeforeNetwork(t *testing.T) {
	c, writes := conditionalServer(t, false)
	r := noticeState(t)
	s := noticeStore(t, c, r)
	base, e := s.Create(budget(t), r)
	if e != nil {
		t.Fatal(e)
	}
	candidate := holdState(t, base.Record(), delivery.ReasonOperator)
	newer, e := s.Save(budget(t), base, candidate)
	if e != nil {
		t.Fatal(e)
	}
	other := noticeStore(t, c, r)
	for _, tt := range []struct {
		s        *NoticeStore
		previous StoredNotice
		next     noticeledger.Record
	}{{s, newer, candidate}, {other, base, candidate}, {s, StoredNotice{}, candidate}, {s, base, r}, {s, base, noticeledger.Record{}}, {nil, base, candidate}, {&NoticeStore{}, base, candidate}} {
		got, e := tt.s.Save(budget(t), tt.previous, tt.next)
		wantError(t, e, InvalidRequest, 0, false)
		if got.Record().Snapshot().NoticeID != "" {
			t.Fatal("failed save returned persisted handle")
		}
	}
	if writes.Load() != 2 {
		t.Fatal("invalid proposal reached storage")
	}
	wrong, e := NewNoticeStore(c, strings.Repeat("f", 64))
	if e != nil {
		t.Fatal(e)
	}
	_, e = wrong.Create(budget(t), r)
	wantError(t, e, InvalidRequest, 0, false)
	for _, client := range []*Client{nil, {}} {
		if _, e := NewNoticeStore(client, strings.Repeat("a", 64)); e == nil {
			t.Fatal("unusable client accepted")
		}
	}
	if _, e := NewNoticeStore(c, "bad"); e == nil {
		t.Fatal("invalid environment accepted")
	}
}

func TestNoticeStoreDoesNotTurnMissingOrCorruptDataIntoEmptyState(t *testing.T) {
	c, writes := conditionalServer(t, false)
	r := noticeState(t)
	s := noticeStore(t, c, r)
	got, e := s.Load(budget(t), stateNoticeID)
	wantError(t, e, Service, 404, false)
	if got.Record().Snapshot().NoticeID != "" || writes.Load() != 0 {
		t.Fatal("404 became new state")
	}
	if _, e := c.Create(budget(t), "state", []byte(`{"private":"not a record"}`)); e != nil {
		t.Fatal(e)
	}
	got, e = s.Load(budget(t), stateNoticeID)
	wantError(t, e, InvalidResponse, 200, false)
	if got.Record().Snapshot().NoticeID != "" || strings.Contains(e.Error(), "private") {
		t.Fatal("corrupt state escaped")
	}
	_, e = s.Load(budget(t), "../../arbitrary")
	wantError(t, e, InvalidRequest, 0, false)
	_, e = (*NoticeStore)(nil).Load(budget(t), stateNoticeID)
	wantError(t, e, InvalidRequest, 0, false)
	_, e = (*NoticeStore)(nil).Create(budget(t), r)
	wantError(t, e, InvalidRequest, 0, false)
}

func TestNoticeStoreUsesHashedNoticePath(t *testing.T) {
	r := noticeState(t)
	want := fmt.Sprintf("/n/example/b/ledger/o/notices%%2F%x.json", sha256.Sum256([]byte(stateNoticeID)))
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != want {
			t.Error("arbitrary object path")
		}
		w.Header().Set("ETag", "tag")
	})
	if _, e := noticeStore(t, c, r).Create(budget(t), r); e != nil {
		t.Fatal(e)
	}
}
