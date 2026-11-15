package ocimonitoring

import (
	"encoding/json"
	"time"
)

type datapoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     int       `json:"value"`
}

type metric struct {
	Namespace     string            `json:"namespace"`
	CompartmentID string            `json:"compartmentId"`
	Name          string            `json:"name"`
	Dimensions    map[string]string `json:"dimensions"`
	Datapoints    []datapoint       `json:"datapoints"`
}

func (c *Client) payload(o Observation, now time.Time) ([]byte, error) {
	if o.At.IsZero() || !o.At.After(now.Add(-2*time.Hour)) || !o.At.Before(now.Add(10*time.Minute)) || o.ReviewIssues != nil && (*o.ReviewIssues < 0 || *o.ReviewIssues > 1000) {
		return nil, &Error{Code: "invalid_observation"}
	}
	var items []metric
	add := func(name string, value int) {
		items = append(items, metric{Namespace: "maintenance_router", CompartmentID: c.compartment,
			Name: name, Dimensions: map[string]string{"installationId": c.installation},
			Datapoints: []datapoint{{Timestamp: o.At.UTC(), Value: value}}})
	}
	if o.CompletedScan {
		add("CompletedScan", 1)
	}
	if o.ReviewIssues != nil {
		add("ReviewIssues", *o.ReviewIssues)
	}
	if o.OperatorReportFailed != nil {
		value := 0
		if *o.OperatorReportFailed {
			value = 1
		}
		add("OperatorReportFailed", value)
	}
	if len(items) == 0 {
		return nil, &Error{Code: "invalid_observation"}
	}
	return json.Marshal(struct {
		BatchAtomicity string   `json:"batchAtomicity"`
		MetricData     []metric `json:"metricData"`
	}{"ATOMIC", items})
}
