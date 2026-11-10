package ocistorage

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRecordOnlyReadPreservesCompleteValidatedRecord(t *testing.T) {
	r := noticeState(t)
	body, _ := r.Encode()
	calls := 0
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, noticeObjectName(stateNoticeID)) {
			t.Error("not one exact read")
		}
		w.Header().Set("ETag", "current")
		_, _ = w.Write(body)
	})
	got, err := noticeStore(t, c, r).LoadRecord(budget(t), noticeObjectName(stateNoticeID))
	encoded, _ := got.Encode()
	if err != nil || !bytes.Equal(encoded, body) || calls != 1 {
		t.Fatalf("record projection lost validation: %v calls=%d", err, calls)
	}
}

func TestRecordOnlyReadPreservesBindingFailures(t *testing.T) {
	r := noticeState(t)
	original, _ := r.Encode()
	env, _ := r.Identity()
	for _, kind := range []string{"path", "environment", "canonical", "missing"} {
		t.Run(kind, func(t *testing.T) {
			body := string(original)
			switch kind {
			case "path":
				body = strings.ReplaceAll(body, stateNoticeID, stateNoticeID+"other")
			case "environment":
				body = strings.ReplaceAll(body, env, strings.Repeat("f", 64))
			case "canonical":
				body += " "
			}
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet {
					t.Error("unexpected write")
				}
				if kind == "missing" {
					w.WriteHeader(404)
					return
				}
				w.Header().Set("ETag", "current")
				_, _ = w.Write([]byte(body))
			})
			got, err := noticeStore(t, c, r).LoadRecord(budget(t), noticeObjectName(stateNoticeID))
			code, status := InvalidResponse, 200
			if kind == "missing" {
				code, status = Service, 404
			}
			wantError(t, err, code, status, false)
			if environment, id := got.Identity(); environment != "" || id != "" {
				t.Fatal("invalid record returned")
			}
		})
	}
	_, err := (*NoticeStore)(nil).LoadRecord(budget(t), noticeObjectName(stateNoticeID))
	wantError(t, err, InvalidRequest, 0, false)
}
