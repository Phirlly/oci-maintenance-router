package ocistorage

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestHistoryPaginationTrailersNeverEstablishAbsence(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"declared", "empty", "undeclared"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if kind != "undeclared" {
					w.Header().Set("Trailer", "opc-next-page")
				}
				_, _ = io.WriteString(w, `{"items":[]}`)
				w.(http.Flusher).Flush()
				switch kind {
				case "declared":
					w.Header().Set("opc-next-page", "more")
				case "undeclared":
					w.Header().Set(http.TrailerPrefix+"Opc-Next-Page", "more")
				}
			})
			empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
			wantError(t, err, InvalidResponse, 200, false)
			if empty || calls.Load() != 1 {
				t.Fatal("late pagination accepted or followed")
			}
		})
	}
}

func TestHistoryUnrelatedTrailerDoesNotHideCompleteEmptyResult(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trailer", "X-Synthetic-Timing")
		_, _ = io.WriteString(w, `{"items":[]}`)
		w.(http.Flusher).Flush()
		w.Header().Set("X-Synthetic-Timing", "1")
	})
	if empty, err := c.HistoryIsEmpty(budget(t), "control/head.json"); err != nil || !empty {
		t.Fatalf("unrelated trailer rejected complete result: %v, %v", empty, err)
	}
}
