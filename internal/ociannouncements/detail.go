package ociannouncements

import "oci-maintenance-router/internal/notices"

func decodeNotice(data []byte, id string) (notices.Notice, error) {
	value, err := strictJSON(data)
	if err != nil {
		return notices.Notice{}, err
	}
	valid := true
	f := objectFields(value, &valid, "id", "announcementType", "chainId", "referenceTicketNumber", "services", "platformType",
		"summary", "description", "additionalInformation", "timeOneType", "timeOneTitle", "timeOneValue",
		"timeTwoType", "timeTwoTitle", "timeTwoValue", "timeCreated", "timeUpdated", "lifecycleState", "affectedResources")
	r := notices.Record{
		NoticeID: f.nonempty("id"), AnnouncementType: f.nonempty("announcementType"),
		ChainID: f.text("chainId", false), Reference: f.text("referenceTicketNumber", false),
		Platform: f.text("platformType", false), Summary: f.text("summary", false), Description: f.text("description", false),
		AdditionalInformation: f.text("additionalInformation", false),
		TimeCreated:           f.text("timeCreated", false), TimeUpdated: f.text("timeUpdated", false), LifecycleState: f.text("lifecycleState", false),
	}
	for i, prefix := range []string{"timeOne", "timeTwo"} {
		r.Times[i] = notices.SemanticTime{Type: f.text(prefix+"Type", false), Title: f.text(prefix+"Title", false), Value: f.text(prefix+"Value", false)}
	}
	for _, value := range f.array("services", false) {
		service, ok := value.(string)
		if !ok {
			valid = false
		}
		r.Services = append(r.Services, service)
	}
	r.Resources = decodeResources(f.array("affectedResources", false), &valid)
	if !valid || r.NoticeID != id {
		return notices.Notice{}, &Error{Code: InvalidResponse}
	}
	notice, err := notices.Normalize(r)
	if err != nil {
		code := InvalidResponse
		if e, ok := err.(*notices.Error); ok && (e.Code == "too_large" || e.Code == "too_many") {
			code = TooLarge
		}
		return notices.Notice{}, &Error{Code: code}
	}
	return notice, nil
}

func decodeResources(values []any, valid *bool) []notices.Resource {
	var resources []notices.Resource
	for _, value := range values {
		f := objectFields(value, valid, "resourceId", "resourceName", "region", "additionalProperties")
		r := notices.Resource{ResourceID: f.nonempty("resourceId"), ResourceName: f.text("resourceName", false), RegionDisplay: f.text("region", false)}
		for _, property := range f.array("additionalProperties", false) {
			p := objectFields(property, valid, "name", "value")
			r.AdditionalProperties = append(r.AdditionalProperties, notices.Property{Name: p.text("name", true), Value: p.text("value", true)})
		}
		resources = append(resources, r)
	}
	return resources
}
