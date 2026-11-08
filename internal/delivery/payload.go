package delivery

import (
	"encoding/json"
	"fmt"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func payload(r notices.Record, revision notices.RevisionKey, environmentName, dedupKey string, resources []routing.Target) []byte {
	times := make([]map[string]any, 0, len(r.Times))
	for i, value := range r.Times {
		times = append(times, map[string]any{"slot": i + 1, "type": value.Type, "title": value.Title, "value": value.Value})
	}
	targets := make([]map[string]string, 0, len(resources))
	for _, resource := range resources {
		targets = append(targets, map[string]string{"resourceId": resource.ResourceID, "region": resource.Region})
	}
	// Reference and time labels are shared source context. This projection drops
	// raw bodies/properties; it cannot redact arbitrary text within shared fields.
	p := map[string]any{
		"summary": fmt.Sprintf("OCI MySQL scheduled maintenance: %d resource(s)", len(resources)),
		"source":  "oci-maintenance-router/" + environmentName, "severity": "info",
		"custom_details": map[string]any{
			"noticeId": revision.NoticeID, "reference": r.Reference,
			"revisionDigest": revision.Digest, "times": times, "resources": targets,
		},
	}
	if r.TimeUpdated != nil {
		p["timestamp"] = r.TimeUpdated
	} else if r.TimeCreated != nil {
		p["timestamp"] = r.TimeCreated
	}
	// This private projection has only JSON-supported types, no caller-supplied
	// values or marshalers. Go-v1 sorted map keys and default escaping are stable.
	data, _ := json.Marshal(map[string]any{"dedup_key": dedupKey, "event_action": "trigger", "payload": p})
	return data
}
