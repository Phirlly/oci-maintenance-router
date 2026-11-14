package ocistorage

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestNoticeListingUsesSignedFixedPrefixAndInclusiveContinuation(t *testing.T) {
	key := generatedKey(t)
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/n/example/b/ledger/o" || !r.Close || r.ProtoMajor != 1 {
			t.Error("wrong listing request")
		}
		want := url.Values{"prefix": {"notices/"}, "limit": {"1000"}}
		if n == 2 {
			want.Set("start", listedName(5))
		}
		if !reflect.DeepEqual(r.URL.Query(), want) {
			t.Error("wrong query", r.URL.RawQuery)
		}
		verifySignature(t, r, key)
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("If-Match") != "" || r.Header.Get("Range") != "" {
			t.Error("wrong headers")
		}
		w.Header().Set("opc-next-page", "must-not-be-used")
		if n == 1 {
			_, _ = w.Write(pageJSON(nil, listedName(5)))
		} else {
			_, _ = w.Write(pageJSON([]string{listedName(5)}, ""))
		}
	})
	s := noticeStore(t, c, noticeState(t))
	first, err := s.ListPage(budget(t), "")
	if err != nil || len(first.Names) != 0 || first.NextStartWith != listedName(5) || calls.Load() != 1 {
		t.Fatal("lost empty page continuation", err)
	}
	second, err := s.ListPage(budget(t), first.NextStartWith)
	if err != nil || !reflect.DeepEqual(second.Names, []string{listedName(5)}) || second.NextStartWith != "" || calls.Load() != 2 {
		t.Fatal("incorrect continuation", err)
	}
}

func TestInvalidDiscoveryInputsDoNotSign(t *testing.T) {
	c, _ := tlsClient(t, unusedProvider{}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	s := noticeStore(t, c, noticeState(t))
	for _, name := range []string{"configuration/router.json", "notices/x.json", listedName(1) + "?x", listedName(1) + "/", "notices/" + string(make([]byte, 64)) + ".json"} {
		p, err := s.ListPage(budget(t), name)
		wantError(t, err, InvalidRequest, 0, false)
		emptyPage(t, p)
		obj, err := s.LoadObject(budget(t), name)
		wantError(t, err, InvalidRequest, 0, false)
		if obj.store != nil {
			t.Fatal("invalid load returned handle")
		}
	}
	for _, store := range []*NoticeStore{nil, {}, {objects: &Client{}, environmentID: "x"}} {
		p, err := store.ListPage(budget(t), "")
		wantError(t, err, InvalidRequest, 0, false)
		emptyPage(t, p)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		p, err := s.ListPage(ctx, "")
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		wantError(t, err, code, 0, false)
		emptyPage(t, p)
	}
	ctx, cancel := context.WithCancel(budget(t))
	cancel()
	p, err := s.ListPage(ctx, "")
	wantError(t, err, Canceled, 0, false)
	emptyPage(t, p)
}

func TestListingFailuresNeverBecomeSuccessfulEmptyPages(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{201, 206, 301, 304, 401, 403, 404, 412, 429, 500} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "://invalid-location")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "private response")
		})
		p, err := noticeStore(t, c, noticeState(t)).ListPage(budget(t), "")
		wantError(t, err, Service, status, false)
		emptyPage(t, p)
		if calls.Load() != 1 {
			t.Fatal("read retried")
		}
	}
}
