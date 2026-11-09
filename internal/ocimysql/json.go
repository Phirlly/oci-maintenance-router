package ocimysql

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const bodyLimit = 2 << 20
const valueLimit = 65536

func strictJSON(data []byte) (any, error) {
	if len(data) > bodyLimit {
		return nil, &Error{Code: TooLarge}
	}
	if !utf8.Valid(data) || !json.Valid(data) || !pairedSurrogates(data) {
		return nil, &Error{Code: InvalidResponse}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	remaining := valueLimit
	return jsonValue(decoder, 0, &remaining)
}

// Syntax and trailing input have already passed json.Valid. Tokens retain
// numbers and detect duplicate names after unescaping, including ignored data.
func jsonValue(decoder *json.Decoder, depth int, remaining *int) (any, error) {
	if *remaining == 0 {
		return nil, &Error{Code: TooLarge}
	}
	*remaining--
	token, err := decoder.Token()
	if err != nil {
		return nil, &Error{Code: InvalidResponse}
	}
	delim, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	if depth >= 32 {
		return nil, &Error{Code: TooLarge}
	}
	if delim == '{' {
		object := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, &Error{Code: InvalidResponse}
			}
			key := keyToken.(string)
			if _, exists := object[key]; exists {
				return nil, &Error{Code: InvalidResponse}
			}
			value, err := jsonValue(decoder, depth+1, remaining)
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
		value, err := jsonValue(decoder, depth+1, remaining)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	_, err = decoder.Token()
	return array, err
}
