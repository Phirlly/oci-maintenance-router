package ocistorage

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const listingLimit = objectLimit
const listingValueLimit = 65536

func listingJSON(data []byte) (any, error) {
	if len(data) > listingLimit {
		return nil, &Error{Code: TooLarge}
	}
	if !utf8.Valid(data) || !json.Valid(data) || !listingPairedSurrogates(data) {
		return nil, &Error{Code: InvalidResponse}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	remaining := listingValueLimit
	return listingJSONValue(decoder, 0, &remaining)
}

// Syntax and trailing input have already passed json.Valid. Tokens retain
// numbers and detect duplicate names after unescaping, including ignored data.
func listingJSONValue(decoder *json.Decoder, depth int, remaining *int) (any, error) {
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
			value, err := listingJSONValue(decoder, depth+1, remaining)
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
		value, err := listingJSONValue(decoder, depth+1, remaining)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	_, err = decoder.Token()
	return array, err
}
