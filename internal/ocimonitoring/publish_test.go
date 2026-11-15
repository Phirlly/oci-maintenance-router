package ocimonitoring

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestPublishSignsOnlyBoundedHistoricalMetrics(t *testing.T) {
	key := generatedKey(t)
	calls := 0
	at := time.Now().UTC().Truncate(time.Millisecond)
	issues, failed := 7, true
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		verifySignature(t, r, key, body)
		if r.Method != "POST" || r.URL.RequestURI() != "/20180401/metrics" || r.Host != "telemetry-ingestion.us-ashburn-1.oraclecloud.com" || r.ProtoMajor != 1 || !r.Close {
			t.Error("wrong signed request boundary")
		}
		var value map[string]json.RawMessage
		if json.Unmarshal(body, &value) != nil || len(value) != 2 || string(value["batchAtomicity"]) != `"ATOMIC"` {
			t.Error("invalid atomic batch", string(body))
		}
		var metrics []struct {
			Name, Namespace, CompartmentID string
			Dimensions                     map[string]string
			Datapoints                     []struct {
				Timestamp time.Time
				Value     float64
			}
			ResourceGroup *string
		}
		if json.Unmarshal(value["metricData"], &metrics) != nil || len(metrics) != 3 {
			t.Error("wrong metric count")
			return
		}
		for i, want := range []struct {
			name  string
			value float64
		}{{"CompletedScan", 1}, {"ReviewIssues", 7}, {"OperatorReportFailed", 1}} {
			m := metrics[i]
			if m.Name != want.name || m.Namespace != "maintenance_router" || m.CompartmentID != compartment || m.ResourceGroup != nil || len(m.Dimensions) != 1 || m.Dimensions["installationId"] != "installation-1" || len(m.Datapoints) != 1 || !m.Datapoints[0].Timestamp.Equal(at) || m.Datapoints[0].Value != want.value {
				t.Errorf("incorrect metric %+v", m)
			}
		}
		io.WriteString(w, `{"failedMetricsCount":0,"failedMetrics":[]}`)
	})
	if err := c.Publish(budget(t), Observation{At: at, CompletedScan: true, ReviewIssues: &issues, OperatorReportFailed: &failed}); err != nil || calls != 1 {
		t.Fatalf("publication err=%v calls=%d", err, calls)
	}
}

func TestUnknownGaugesAreOmittedAndKnownZeroIsPublished(t *testing.T) {
	issues, failed := 0, false
	for _, tc := range []struct {
		name        string
		observation Observation
		expected    string
	}{
		{"completed-only", Observation{CompletedScan: true}, `"CompletedScan"`},
		{"known-empty", Observation{ReviewIssues: &issues}, `"ReviewIssues"`},
		{"recovered", Observation{OperatorReportFailed: &failed}, `"OperatorReportFailed"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
				var body struct{ MetricData []map[string]json.RawMessage }
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.MetricData) != 1 || string(body.MetricData[0]["name"]) != tc.expected {
					t.Error("unknown metric was fabricated")
				} else {
					var points []struct{ Value int }
					want := 0
					if tc.name == "completed-only" {
						want = 1
					}
					if json.Unmarshal(body.MetricData[0]["datapoints"], &points) != nil || len(points) != 1 || points[0].Value != want {
						t.Error("known zero or completed count changed")
					}
				}
				io.WriteString(w, `{"failedMetricsCount":0}`)
			})
			tc.observation.At = time.Now()
			if err := c.Publish(budget(t), tc.observation); err != nil {
				t.Fatal(err)
			}
		})
	}
}
