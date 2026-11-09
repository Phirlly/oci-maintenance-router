package ocisecrets

import (
	"bytes"
	"encoding/json"
	"io"
)

// Preserve case-exact field identity and reject duplicate members. Unrelated
// optional metadata is ignored without importing SDK model logging/formatting.
func bundleObject(body []byte) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, false
		}
		// Within an object Token rejects nonstring keys before returning.
		name := token.(string)
		if _, duplicate := fields[name]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return nil, false
	}
	var trailing json.RawMessage
	if decoder.Decode(&trailing) != io.EOF {
		return nil, false
	}
	return fields, true
}
