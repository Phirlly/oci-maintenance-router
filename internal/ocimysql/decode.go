package ocimysql

import (
	"strings"

	"oci-maintenance-router/internal/routing"
)

func decodeMetadata(data []byte, requestedID, tenancy string) (routing.Metadata, error) {
	value, err := strictJSON(data)
	if err != nil {
		return routing.Metadata{}, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return routing.Metadata{}, &Error{Code: InvalidResponse}
	}
	for key := range object {
		for _, known := range []string{"id", "compartmentId", "definedTags", "freeformTags"} {
			if key != known && strings.EqualFold(key, known) {
				return routing.Metadata{}, &Error{Code: InvalidResponse}
			}
		}
	}
	id, ok := object["id"].(string)
	if !ok || id == "" || id != requestedID {
		return routing.Metadata{}, &Error{Code: InvalidResponse}
	}
	compartment, ok := object["compartmentId"].(string)
	if !ok || len(compartment) > 4096 || (compartment != tenancy && !compartmentPattern.MatchString(compartment)) {
		return routing.Metadata{}, &Error{Code: InvalidResponse}
	}
	defined, err := definedTags(object["definedTags"])
	if err != nil {
		return routing.Metadata{}, err
	}
	freeform, err := freeformTags(object["freeformTags"])
	if err != nil {
		return routing.Metadata{}, err
	}
	return routing.Metadata{ID: &id, CompartmentID: &compartment, DefinedTags: defined, FreeformTags: freeform}, nil
}

func tagObject(value any, optional bool) (map[string]any, error) {
	if value == nil && optional {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, &Error{Code: InvalidResponse}
	}
	return object, nil
}

func definedTags(value any) (map[string]map[string]any, error) {
	object, err := tagObject(value, true)
	if err != nil || object == nil {
		return nil, err
	}
	result := make(map[string]map[string]any, len(object))
	for namespace, value := range object {
		tags, err := tagObject(value, false)
		if err != nil {
			return nil, err
		}
		// Keep exact names and all value types. Routing owns selection, case
		// ambiguity and nonstring rejection; no coercion or fallback occurs here.
		result[namespace] = tags
	}
	return result, nil
}

func freeformTags(value any) (map[string]string, error) {
	object, err := tagObject(value, true)
	if err != nil || object == nil {
		return nil, err
	}
	result := make(map[string]string, len(object))
	for name, value := range object {
		text, ok := value.(string)
		if !ok {
			return nil, &Error{Code: InvalidResponse}
		}
		result[name] = text
	}
	return result, nil
}
