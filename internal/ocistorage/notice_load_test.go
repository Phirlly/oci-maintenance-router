package ocistorage

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestDiscoveredNoticeLoadsWithoutAnnouncementAndKeepsCASBinding(t *testing.T) {
	c, _ := conditionalServer(t, false)
	r := noticeState(t)
	s := noticeStore(t, c, r)
	if _, err := s.Create(budget(t), r); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadObject(budget(t), noticeObjectName(stateNoticeID))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := r.Encode()
	got, _ := loaded.Record().Encode()
	if !bytes.Equal(got, want) || loaded.etag == "" || loaded.store != s {
		t.Fatal("record or binding lost")
	}
	next := holdState(t, loaded.Record(), delivery.ReasonOperator)
	saved, err := s.Save(budget(t), loaded, next)
	if err != nil || saved.Record().Snapshot().HoldReason != delivery.ReasonOperator {
		t.Fatal("discovered handle cannot save", err)
	}
	_, err = s.Save(budget(t), loaded, next)
	wantError(t, err, Conflict, 412, false)
	newer, err := s.LoadObject(budget(t), noticeObjectName(stateNoticeID))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(budget(t), newer, next)
	wantError(t, err, InvalidRequest, 0, false)
}

func TestDiscoveredNoticeRejectsWrongPathEnvironmentAndNoncanonicalRecord(t *testing.T) {
	r := noticeState(t)
	original, _ := r.Encode()
	env, _ := r.Identity()
	for _, kind := range []string{"malformed", "null", "no id", "wrong type", "wrong hash", "bad id", "foreign environment", "noncanonical", "duplicate", "alias", "invalid ledger"} {
		t.Run(kind, func(t *testing.T) {
			body := string(original)
			switch kind {
			case "malformed":
				body = "{"
			case "null":
				body = "null"
			case "no id":
				body = "{}"
			case "wrong type":
				body = strings.Replace(body, `"noticeId":"`+stateNoticeID+`"`, `"noticeId":1`, 1)
			case "wrong hash":
				body = strings.ReplaceAll(body, stateNoticeID, stateNoticeID+"other")
			case "bad id":
				body = strings.ReplaceAll(body, stateNoticeID, "not-an-announcement")
			case "foreign environment":
				body = strings.ReplaceAll(body, env, strings.Repeat("f", 64))
			case "noncanonical":
				body += " "
			case "duplicate":
				body = strings.TrimSuffix(body, "}") + `,"noticeId":"` + stateNoticeID + `"}`
			case "alias":
				body = strings.Replace(body, `"noticeId":`, `"NoticeId":`, 1)
			case "invalid ledger":
				body = strings.Replace(body, `"schemaVersion":2`, `"schemaVersion":3`, 1)
			}
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("ETag", "current")
				_, _ = w.Write([]byte(body))
			})
			obj, err := noticeStore(t, c, r).LoadObject(budget(t), noticeObjectName(stateNoticeID))
			wantError(t, err, InvalidResponse, 200, false)
			if obj.store != nil || obj.etag != "" || obj.Record().Snapshot().NoticeID != "" {
				t.Fatal("invalid saved data returned")
			}
		})
	}
}

func TestUnavailableDiscoveredNoticeIsNotNewState(t *testing.T) {
	c, writes := conditionalServer(t, false)
	s := noticeStore(t, c, noticeState(t))
	obj, err := s.LoadObject(budget(t), noticeObjectName(stateNoticeID))
	wantError(t, err, Service, 404, false)
	if obj.store != nil || writes.Load() != 0 {
		t.Fatal("absence inferred or record created")
	}
	for _, store := range []*NoticeStore{nil, {}} {
		_, err := store.LoadObject(budget(t), noticeObjectName(stateNoticeID))
		wantError(t, err, InvalidRequest, 0, false)
	}
}
