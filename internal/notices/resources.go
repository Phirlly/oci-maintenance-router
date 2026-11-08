package notices

import (
	"reflect"
	"slices"
	"strings"
)

// The caller owns the copy and has already validated required property pointers.
func normalizeResources(resources []Resource) ([]Resource, error) {
	for i := range resources {
		slices.SortFunc(resources[i].AdditionalProperties, func(a, b Property) int {
			if order := strings.Compare(*a.Name, *b.Name); order != 0 {
				return order
			}
			return strings.Compare(*a.Value, *b.Value)
		})
	}
	slices.SortFunc(resources, func(a, b Resource) int { return strings.Compare(a.ResourceID, b.ResourceID) })
	unique := resources[:0]
	for _, resource := range resources {
		if len(unique) > 0 && unique[len(unique)-1].ResourceID == resource.ResourceID {
			if !reflect.DeepEqual(unique[len(unique)-1], resource) {
				return nil, &Error{"resources", "conflicting_duplicate"}
			}
			continue
		}
		if len(unique) == maxResources {
			return nil, &Error{"resources", "too_many"}
		}
		unique = append(unique, resource)
	}
	return unique, nil
}
