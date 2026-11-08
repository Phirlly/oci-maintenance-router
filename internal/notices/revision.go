package notices

import "encoding/json"

// Only this closed set of strings, pointers, integers, arrays and maps is encoded.
// No raw SDK objects, arbitrary values or MarshalJSON methods enter the digest.
func projection(r Record, includeContext bool) []byte {
	times := make([]map[string]any, 0, 2)
	for i, slot := range r.Times {
		times = append(times, map[string]any{"slot": i + 1, "type": slot.Type, "title": slot.Title, "value": slot.Value})
	}
	resources := make([]map[string]any, 0, len(r.Resources))
	for _, resource := range r.Resources {
		properties := make([]map[string]any, 0, len(resource.AdditionalProperties))
		for _, property := range resource.AdditionalProperties {
			properties = append(properties, map[string]any{"name": property.Name, "value": property.Value})
		}
		resources = append(resources, map[string]any{
			"resourceId": resource.ResourceID, "resourceName": resource.ResourceName,
			"regionDisplay": resource.RegionDisplay, "additionalProperties": properties,
		})
	}
	services := r.Services
	if services == nil {
		services = []string{}
	}
	value := map[string]any{
		"schemaVersion": 1, "noticeId": r.NoticeID, "chainId": r.ChainID, "reference": r.Reference,
		"services": services, "platform": r.Platform, "announcementType": r.AnnouncementType,
		"summary": r.Summary, "description": r.Description, "additionalInformation": r.AdditionalInformation,
		"times": times, "resources": resources,
	}
	if includeContext {
		value["timeCreated"], value["timeUpdated"] = r.TimeCreated, r.TimeUpdated
		value["lifecycleState"] = r.LifecycleState
	}
	// The fixed projection above contains no unsupported values or cycles.
	encoded, _ := json.Marshal(value)
	return encoded
}
