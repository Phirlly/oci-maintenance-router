package ociannouncements

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentReadsOwnTheirResults(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/"+noticeID) {
			io.WriteString(w, detailed)
		} else {
			fmt.Fprintf(w, `{"items":[{"id":%q}]}`, noticeID)
		}
	})
	ctx := budget(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			n, err := c.Get(ctx, noticeID)
			if err != nil {
				t.Error(err)
				return
			}
			r := n.Snapshot()
			*r.Summary = "caller change"
			if *n.Snapshot().Summary != "Synthetic scheduled maintenance" {
				t.Error("notice aliases caller")
			}
			p, err := c.ListPage(ctx, "")
			if err != nil || len(p.IDs) != 1 || p.IDs[0] != noticeID {
				t.Error("concurrent page corrupted")
				return
			}
			p.IDs[0] = "caller change"
		})
	}
	wg.Wait()
}
