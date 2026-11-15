package ocimonitoring

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicationRequiresCompleteUnambiguousAcceptance(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`, `[]`, `{"failedMetricsCount":null}`, `{"failedMetricsCount":"0"}`,
		`{"failedMetricsCount":0.5}`, `{"failedMetricsCount":-1}`, `{"failedMetricsCount":1}`,
		`{"failedMetricsCount":0,"failedMetrics":[{}]}`, `{"failedMetricsCount":0,"failedMetrics":{}}`,
		`{"failedMetricsCount":1,"failedMetricsCount":0}`, `{"failedMetricsCount":0,"failed\u004detricsCount":0}`,
		`{"failedMetricsCount":0} {}`, `{"failedMetricsCount":0,"unexpected":"private-data"}`, "invalid-private-data",
	} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
				calls++
				io.WriteString(w, body)
			})
			err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true})
			if err == nil || calls != 1 || strings.Contains(err.Error(), "private-data") {
				t.Fatalf("unsafe acceptance/retry/error: %v calls=%d", err, calls)
			}
		})
	}
}

func TestPublicationRejectsStatusAndRepresentationChanges(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		header, value string
	}{
		{"accepted-status", 202, "", ""}, {"redirect", 307, "Location", controlOrigin},
		{"throttled", 429, "Retry-After", "1"}, {"service-error", 503, "", ""},
		{"range", 200, "Content-Range", "bytes 0-4/10"}, {"encoding", 200, "Content-Encoding", "gzip"},
		{"large", 200, "Content-Length", "20000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.header != "" {
					w.Header().Set(tc.header, tc.value)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, `{"failedMetricsCount":0}`)
			})
			if err := c.Publish(budget(t), Observation{At: time.Now(), CompletedScan: true}); err == nil || calls != 1 {
				t.Fatalf("unsafe representation/retry: %v calls=%d", err, calls)
			}
		})
	}
}
