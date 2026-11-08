package configuration

import "slices"

type objectReader struct {
	values map[string]any
	path   string
	err    *error
}

func readObject(value any, path string, err *error, required, optional []string) objectReader {
	values, ok := value.(map[string]any)
	reader := objectReader{values, path, err}
	if !ok {
		reader.fail(path, "type")
		return reader
	}
	for _, field := range required {
		if _, exists := values[field]; !exists {
			reader.fail(reader.field(field), "required")
		}
	}
	for field := range values {
		if !slices.Contains(required, field) && !slices.Contains(optional, field) {
			reader.fail(path, "unknown_field")
		}
	}
	return reader
}

func (r objectReader) field(name string) string {
	if r.path == "configuration" {
		return name
	}
	return r.path + "." + name
}

func (r objectReader) fail(field, code string) {
	if *r.err == nil {
		*r.err = &Error{field, code}
	}
}

func (r objectReader) text(name string) string {
	value, ok := r.values[name].(string)
	if !ok {
		r.fail(r.field(name), "type")
	}
	return value
}

func (r objectReader) array(name string) []any {
	value, ok := r.values[name].([]any)
	if !ok {
		r.fail(r.field(name), "type")
	}
	return value
}

func (r objectReader) strings(name string) []string {
	values := r.array(name)
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value, ok := raw.(string)
		if !ok {
			r.fail(r.field(name), "type")
		}
		result = append(result, value)
	}
	return result
}

func (r objectReader) stringMap(name string) map[string]string {
	values, ok := r.values[name].(map[string]any)
	if !ok {
		r.fail(r.field(name), "type")
	}
	result := make(map[string]string, len(values))
	for key, raw := range values {
		value, ok := raw.(string)
		if !ok {
			r.fail(r.field(name), "type")
		}
		result[key] = value
	}
	return result
}
