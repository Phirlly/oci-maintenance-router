package ocistorage

import (
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestHistoryEmptyUsesOneSignedVersionPrefixRequest(t *testing.T) {
	key := generatedKey(t)
	name := "control/head ?#%/雪.json"
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/n/example/b/ledger/objectversions" || !r.Close || r.ProtoMajor != 1 {
			t.Error("incorrect version-history request")
		}
		if !reflect.DeepEqual(r.URL.Query(), url.Values{"prefix": {name}, "limit": {"1"}}) {
			t.Error("incorrect version-history query")
		}
		verifySignature(t, r, key)
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("If-Match") != "" || r.Header.Get("Range") != "" {
			t.Error("unexpected history headers")
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Error("history request included a body")
		}
		_, _ = io.WriteString(w, `{"items":[]}`)
	})
	empty, err := c.HistoryIsEmpty(budget(t), name)
	if err != nil || !empty || calls.Load() != 1 {
		t.Fatalf("complete empty history = %v, %v; requests=%d", empty, err, calls.Load())
	}
}
