package pagerduty

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// Local wire budget, including the key; delivery limits saved envelopes to32KiB.
const requestLimit = 33 << 10

func envelope(saved, key []byte) ([]byte, error) {
	if len(saved) > requestLimit {
		return nil, &Error{Code: TooLarge}
	}
	if len(key) < 32 || len(key) > 128 || !utf8.Valid(key) || utf8.RuneCount(key) != 32 ||
		len(saved) < 2 || saved[0] != '{' || saved[len(saved)-1] != '}' || !utf8.Valid(saved) || !json.Valid(saved) {
		return nil, &Error{Code: InvalidRequest}
	}
	// Syntax is valid and the root is an object, so tokens are string keys and
	// RawMessage decoding cannot fail. Keep duplicate detection at this boundary.
	dec := json.NewDecoder(bytes.NewReader(saved))
	_, _ = dec.Token()
	fields := make(map[string]json.RawMessage, 3)
	for dec.More() {
		token, _ := dec.Token()
		name := token.(string)
		if (name != "dedup_key" && name != "event_action" && name != "payload") || fields[name] != nil {
			return nil, &Error{Code: InvalidRequest}
		}
		var raw json.RawMessage
		_ = dec.Decode(&raw)
		fields[name] = raw
	}
	var dedup, action string
	payload := bytes.TrimSpace(fields["payload"])
	if len(fields) != 3 || json.Unmarshal(fields["dedup_key"], &dedup) != nil || dedup == "" || utf8.RuneCountInString(dedup) > 255 ||
		json.Unmarshal(fields["event_action"], &action) != nil || action != "trigger" || len(payload) < 2 || payload[0] != '{' {
		return nil, &Error{Code: InvalidRequest}
	}
	encoded, _ := json.Marshal(string(key)) // String marshaling cannot fail.
	prefix := []byte("{\"routing_key\":")
	if len(prefix)+len(encoded)+len(saved) > requestLimit {
		return nil, &Error{Code: TooLarge}
	}
	body := make([]byte, 0, len(prefix)+len(encoded)+len(saved))
	body = append(body, prefix...)
	body = append(body, encoded...)
	body = append(body, ',')
	return append(body, saved[1:]...), nil
}
