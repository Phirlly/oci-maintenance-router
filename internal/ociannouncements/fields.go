package ociannouncements

import "strings"

// Wire readers accept additive fields but reject case aliases and type coercion.
// The shared validity flag keeps nested failures from yielding partial results.
type fields struct {
	object map[string]any
	valid  *bool
}

func objectFields(value any, valid *bool, names ...string) fields {
	object, ok := value.(map[string]any)
	if !ok {
		*valid = false
	}
	for key := range object {
		for _, name := range names {
			if key != name && strings.EqualFold(key, name) {
				*valid = false
			}
		}
	}
	return fields{object, valid}
}

func (f fields) text(name string, required bool) *string {
	value := f.object[name]
	if value == nil && !required {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		*f.valid = false
		return nil
	}
	return &text
}

func (f fields) nonempty(name string) string {
	text := f.text(name, true)
	if text == nil || *text == "" {
		*f.valid = false
		return ""
	}
	return *text
}

func (f fields) array(name string, required bool) []any {
	value := f.object[name]
	if value == nil && !required {
		return nil
	}
	array, ok := value.([]any)
	if !ok {
		*f.valid = false
		return nil
	}
	return array
}
