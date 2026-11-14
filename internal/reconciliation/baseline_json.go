package reconciliation

import (
	"bytes"
	"encoding/json"
	"io"
)

func boundedBaselineJSON(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	entries := 0
	if !baselineJSONValue(d, 0, &entries) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// A single total array-entry budget bounds allocation even for tiny invalid
// elements such as null. Canonical semantic validation follows typed decoding.
func baselineJSONValue(d *json.Decoder, depth int, entries *int) bool {
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, container := token.(json.Delim)
	if !container {
		return true
	}
	if depth >= 32 {
		return false
	}
	close := json.Delim('}')
	if delim == '[' {
		close = ']'
	}
	for d.More() {
		if delim == '{' {
			if _, err := d.Token(); err != nil {
				return false
			}
		} else {
			*entries += 1
			if *entries > baselineEntries {
				return false
			}
		}
		if !baselineJSONValue(d, depth+1, entries) {
			return false
		}
	}
	end, err := d.Token()
	return err == nil && end == close
}
