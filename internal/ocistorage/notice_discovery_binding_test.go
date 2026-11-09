package ocistorage

import (
	"bytes"
	"net/http"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestListedRecordCanChangeBeforeLoadAndUsesCurrentGETVersion(t *testing.T) {
	original := noticeState(t)
	changed := holdState(t, original, delivery.ReasonOperator)
	body, _ := changed.Encode()
	var requestedMatch string
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/n/example/b/ledger/o":
			w.Header().Set("ETag", "listing-not-a-version")
			_, _ = w.Write(pageJSON([]string{noticeObjectName(stateNoticeID)}, ""))
		case r.Method == "GET":
			if r.URL.Path != "/n/example/b/ledger/o/"+noticeObjectName(stateNoticeID) || r.URL.RawQuery != "" {
				t.Error("wrong object request")
			}
			w.Header().Set("ETag", "current-get-version")
			_, _ = w.Write(body)
		case r.Method == "PUT":
			requestedMatch = r.Header.Get("If-Match")
			w.Header().Set("ETag", "saved")
		default:
			t.Error("unexpected operation")
		}
	})
	s := noticeStore(t, c, original)
	page, err := s.ListPage(budget(t), "")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadObject(budget(t), page.Names[0])
	if err != nil {
		t.Fatal(err)
	}
	got, _ := loaded.Record().Encode()
	if !bytes.Equal(got, body) || loaded.Record().Snapshot().HoldReason != delivery.ReasonOperator {
		t.Fatal("load used stale list data")
	}
	next := holdState(t, loaded.Record(), delivery.ReasonOperator)
	if _, err = s.Save(budget(t), loaded, next); err != nil {
		t.Fatal(err)
	}
	if requestedMatch != "current-get-version" {
		t.Fatal("listing ETag used for update")
	}
}
