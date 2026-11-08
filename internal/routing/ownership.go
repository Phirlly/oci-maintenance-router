package routing

import "unicode/utf8"

const maxMetadataText = 1 << 20
const maxDefinedEntries = 256

func (p *Policy) destination(m Metadata) (string, Reason) {
	var value any
	if p.owner.Kind == "freeform" {
		owner, exists := m.FreeformTags[p.owner.Key]
		if !exists {
			return "", ReasonTagMissing
		}
		value = owner
	} else {
		var reason Reason
		value, reason = p.definedValue(m.DefinedTags)
		if reason != "" {
			return "", reason
		}
	}
	owner, ok := value.(string)
	if !ok {
		return "", ReasonTagInvalid
	}
	if len(owner) > maxMetadataText {
		return "", ReasonMetadataLimit
	}
	if owner == "" || !utf8.ValidString(owner) {
		return "", ReasonTagInvalid
	}
	destination, exists := p.routes[owner]
	if !exists {
		return "", ReasonOwnerUnmapped
	}
	return destination, ""
}

func (p *Policy) definedValue(tags map[string]map[string]any) (any, Reason) {
	if len(tags) > maxDefinedEntries {
		return nil, ReasonMetadataLimit
	}
	var selected map[string]any
	found := false
	for namespace, values := range tags {
		if asciiNameEqual(namespace, p.owner.Namespace) {
			if found {
				return nil, ReasonTagAmbiguous
			}
			selected, found = values, true
		}
	}
	if len(selected) > maxDefinedEntries {
		return nil, ReasonMetadataLimit
	}
	found = false
	var value any
	for key, candidate := range selected {
		if asciiNameEqual(key, p.owner.Key) {
			if found {
				return nil, ReasonTagAmbiguous
			}
			value, found = candidate, true
		}
	}
	if !found {
		return nil, ReasonTagMissing
	}
	return value, ""
}

// Configured names are ASCII. Avoid Unicode folding of untrusted metadata names.
func asciiNameEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
