package ocistorage

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestHistoryContinuationNeverEstablishesCompleteAbsence(t *testing.T) {
	key := generatedKey(t)
	for _, tokens := range [][]string{{""}, {"more"}, {"one", "two"}} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			for _, token := range tokens {
				w.Header().Add("opc-next-page", token)
			}
			_, _ = io.WriteString(w, `{"items":[]}`)
		})
		empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
		wantError(t, err, InvalidResponse, 200, false)
		if empty || calls.Load() != 1 {
			t.Fatal("partial listing accepted or followed")
		}
	}
}

func TestHistoryServiceFailuresNeverEstablishAbsenceOrRetry(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{201, 206, 301, 304, 401, 403, 404, 412, 429, 500} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "://invalid-location")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "private response")
		})
		empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
		wantError(t, err, Service, status, false)
		if empty || calls.Load() != 1 || err.Error() != "object storage: service" {
			t.Fatal("service failure escaped, established absence or retried")
		}
	}
}
