package configuration

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const maxConfigBytes = 1 << 20

func strictJSON(data []byte) (any, error) {
	if len(data) > maxConfigBytes {
		return nil, &Error{"configuration", "too_large"}
	}
	if !utf8.Valid(data) || !json.Valid(data) || !pairedSurrogates(data) {
		return nil, &Error{"configuration", "invalid_json"}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return jsonValue(decoder, 0)
}

// json.Valid has already checked syntax and trailing input. Walking tokens
// preserves number spelling and catches duplicate names after JSON unescaping.
func jsonValue(decoder *json.Decoder, depth int) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, &Error{"configuration", "invalid_json"}
	}
	delim, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	if depth >= 32 {
		return nil, &Error{"configuration", "invalid_json"}
	}
	if delim == '{' {
		object := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, &Error{"configuration", "invalid_json"}
			}
			key := keyToken.(string)
			if _, exists := object[key]; exists {
				return nil, &Error{"configuration", "duplicate_key"}
			}
			value, err := jsonValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		_, err = decoder.Token()
		return object, err
	}
	array := []any{}
	for decoder.More() {
		value, err := jsonValue(decoder, depth+1)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	_, err = decoder.Token()
	return array, err
}
