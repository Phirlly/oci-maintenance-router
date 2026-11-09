package ociannouncements

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestSignedRequestContracts(t *testing.T) {
	key := generatedKey(t)
	token := " +/%?é= "
	var requests atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		number := requests.Add(1)
		verifySignature(t, r, key)
		if r.Method != "GET" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Date") == "" || !r.Close || r.ProtoMajor != 1 {
			t.Error("incorrect read request")
		}
		if r.URL.Path == "/20180904/announcements/"+noticeID {
			if r.URL.RawQuery != "" {
				t.Error("detail query changed")
			}
			fmt.Fprint(w, detailed)
		} else {
			if r.URL.Path != "/20180904/announcements" {
				t.Error("wrong list path")
			}
			want := url.Values{"compartmentId": {"ocid1.tenancy.oc1..example"}, "limit": {"100"}, "shouldShowOnlyLatestInChain": {"false"}}
			if number == 3 {
				want.Set("page", token)
			}
			if !reflect.DeepEqual(r.URL.Query(), want) {
				t.Errorf("list query changed: %s", r.URL.RawQuery)
			}
			w.Header().Set("opc-next-page", "next")
			fmt.Fprintf(w, `{"items":[{"id":%q}]}`, noticeID)
		}
	})
	n, err := c.Get(budget(t), noticeID)
	if err != nil || n.Key().NoticeID != noticeID {
		t.Fatalf("detail read failed: %v", err)
	}
	for _, page := range []string{"", token} {
		p, err := c.ListPage(budget(t), page)
		if err != nil || !reflect.DeepEqual(p.IDs, []string{noticeID}) || p.NextPage != "next" {
			t.Fatalf("list read failed: %+v %v", p, err)
		}
	}
	if requests.Load() != 3 {
		t.Fatal("unexpected request count")
	}
}
