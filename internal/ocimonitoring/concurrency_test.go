package ocimonitoring

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func containsMetricValue(body []byte, name string, value int) bool {
	var decoded struct {
		MetricData []struct {
			Name       string
			Datapoints []struct{ Value int }
		}
	}
	if json.Unmarshal(body, &decoded) != nil {
		return false
	}
	for _, metric := range decoded.MetricData {
		if metric.Name == name && len(metric.Datapoints) == 1 && metric.Datapoints[0].Value == value {
			return true
		}
	}
	return false
}

func TestConcurrentMetricCallsKeepIndependentBodies(t *testing.T) {
	var mu sync.Mutex
	seen := map[int]int{}
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for i := 0; i < 8; i++ {
			if containsMetricValue(body, "ReviewIssues", i) {
				mu.Lock()
				seen[i]++
				mu.Unlock()
			}
		}
		io.WriteString(w, `{"failedMetricsCount":0,"failedMetrics":null}`)
	})
	ctx := budget(t)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			count := i
			if err := c.Publish(ctx, Observation{At: time.Now(), ReviewIssues: &count}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 8 {
		t.Fatal("concurrent request lost", seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal("request repeated or mutated", seen)
		}
	}
}
