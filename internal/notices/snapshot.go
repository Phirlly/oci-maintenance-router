package notices

// Snapshot returns an independent copy of source context and normalized content.
// It is not the minimal state record or recipient-safe delivery payload.
func (n Notice) Snapshot() Record { return cloneRecord(n.record) }

func cloneText(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRecord(r Record) Record {
	r.ChainID, r.Reference = cloneText(r.ChainID), cloneText(r.Reference)
	r.Platform, r.Summary = cloneText(r.Platform), cloneText(r.Summary)
	r.Description, r.AdditionalInformation = cloneText(r.Description), cloneText(r.AdditionalInformation)
	r.TimeCreated, r.TimeUpdated = cloneText(r.TimeCreated), cloneText(r.TimeUpdated)
	r.LifecycleState = cloneText(r.LifecycleState)
	r.Services = append([]string{}, r.Services...)
	for i, slot := range r.Times {
		r.Times[i] = SemanticTime{cloneText(slot.Type), cloneText(slot.Title), cloneText(slot.Value)}
	}
	resources := make([]Resource, len(r.Resources))
	for i, resource := range r.Resources {
		resource.ResourceName = cloneText(resource.ResourceName)
		resource.RegionDisplay = cloneText(resource.RegionDisplay)
		properties := make([]Property, len(resource.AdditionalProperties))
		for j, property := range resource.AdditionalProperties {
			properties[j] = Property{cloneText(property.Name), cloneText(property.Value)}
		}
		resource.AdditionalProperties = properties
		resources[i] = resource
	}
	r.Resources = resources
	return r
}
