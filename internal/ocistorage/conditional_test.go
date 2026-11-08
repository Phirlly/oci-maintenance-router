package ocistorage

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// A synthetic implementation of the documented single-object conditional API.
// It verifies the actual adapter's requests; it is not a router coordinator.
func conditionalServer(t *testing.T, dropFirstResponse bool) (*Client, *atomic.Int32) {
	drop := 0
	if dropFirstResponse {
		drop = 1
	}
	return conditionalServerAt(t, drop)
}

func conditionalServerAt(t *testing.T, dropVersion int) (*Client, *atomic.Int32) {
	t.Helper()
	var mu sync.Mutex
	var body []byte
	var version int
	var writes atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "PUT" {
			writes.Add(1)
			candidate, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			create := r.Header.Get("If-None-Match") == "*" && r.Header.Get("If-Match") == ""
			replace := version > 0 && r.Header.Get("If-None-Match") == "" && r.Header.Get("If-Match") == fmt.Sprintf("v%d", version)
			if (!create && !replace) || (create && version > 0) {
				w.WriteHeader(412)
				return
			}
			body = candidate
			version++
			if dropVersion == version {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
		}
		if version == 0 {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", fmt.Sprintf("v%d", version))
		if r.Method == "GET" {
			_, _ = w.Write(body)
		}
	})
	return c, &writes
}

func TestCreateCollisionAndStaleReplacementPreserveStoredBytes(t *testing.T) {
	c, writes := conditionalServer(t, false)
	tag, err := c.Create(budget(t), "state", []byte("first"))
	if err != nil || tag != "v1" {
		t.Fatalf("create: %s %v", tag, err)
	}
	_, err = c.Create(budget(t), "state", []byte("overwrite"))
	wantError(t, err, Conflict, 412, false)
	tag, err = c.Replace(budget(t), "state", tag, []byte("second"))
	if err != nil || tag != "v2" {
		t.Fatalf("replace: %s %v", tag, err)
	}
	_, err = c.Replace(budget(t), "state", "v1", []byte("stale"))
	wantError(t, err, Conflict, 412, false)
	obj, err := c.Get(budget(t), "state")
	if err != nil || string(obj.Body) != "second" || obj.ETag != "v2" || writes.Load() != 4 {
		t.Fatal("conflict overwrote data or retried")
	}
}

func TestCompetingReplacementsHaveOneWinner(t *testing.T) {
	c, writes := conditionalServer(t, false)
	tag, err := c.Create(budget(t), "state", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	type result struct {
		body string
		tag  string
		err  error
	}
	results := make(chan result, 2)
	ctx := budget(t)
	for _, value := range []string{"left", "right"} {
		wg.Go(func() { etag, err := c.Replace(ctx, "state", tag, []byte(value)); results <- result{value, etag, err} })
	}
	wg.Wait()
	close(results)
	winner := ""
	for got := range results {
		if got.err == nil {
			if winner != "" || got.tag != "v2" {
				t.Fatal("multiple winners")
			}
			winner = got.body
		} else {
			wantError(t, got.err, Conflict, 412, false)
		}
	}
	obj, err := c.Get(ctx, "state")
	if winner == "" || err != nil || string(obj.Body) != winner || writes.Load() != 3 {
		t.Fatal("atomic condition not preserved")
	}
}

func TestLostWriteResponseRequiresRereadWithoutRetransmission(t *testing.T) {
	c, writes := conditionalServer(t, true)
	tag, err := c.Create(budget(t), "state", []byte("saved-before-disconnect"))
	wantError(t, err, Transport, 0, true)
	if tag != "" || writes.Load() != 1 {
		t.Fatal("lost response retried or claimed success")
	}
	obj, err := c.Get(budget(t), "state")
	if err != nil || obj.ETag != "v1" || string(obj.Body) != "saved-before-disconnect" {
		t.Fatal("reread did not reveal committed write")
	}
}
