package delivery

import (
	"encoding/json"
	"fmt"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func legacyAnnouncement(category string, resources []routing.Target) bool {
	if category != "SCHEDULED_MAINTENANCE" {
		return false
	}
	for _, resource := range resources {
		if !recordDBID.MatchString(resource.ResourceID) {
			return false
		}
	}
	return true
}

// This independently versioned projection leaves the legacy encoder frozen.
// Shared reference/time text is retained; raw source bodies and tags are omitted.
func announcementPayload(r notices.Record, revision notices.RevisionKey, environmentName, dedupKey string, resources []routing.Target) []byte {
	times := make([]map[string]any, 0, len(r.Times))
	for i, value := range r.Times {
		times = append(times, map[string]any{"slot": i + 1, "type": value.Type, "title": value.Title, "value": value.Value})
	}
	targets := make([]map[string]string, 0, len(resources))
	for _, resource := range resources {
		targets = append(targets, map[string]string{"resourceId": resource.ResourceID, "region": resource.Region})
	}
	p := map[string]any{
		"summary": fmt.Sprintf("OCI announcement: %d resource(s)", len(resources)),
		"source":  "oci-maintenance-router/" + environmentName, "severity": "info",
		"custom_details": map[string]any{
			"formatVersion": 2, "announcementType": r.AnnouncementType,
			"noticeId": revision.NoticeID, "reference": r.Reference,
			"revisionDigest": revision.Digest, "times": times, "resources": targets,
		},
	}
	if r.TimeUpdated != nil {
		p["timestamp"] = r.TimeUpdated
	} else if r.TimeCreated != nil {
		p["timestamp"] = r.TimeCreated
	}
	data, _ := json.Marshal(map[string]any{"dedup_key": dedupKey, "event_action": "trigger", "payload": p})
	return data
}
