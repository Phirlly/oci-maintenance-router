package delivery

import (
	"bytes"
	"encoding/json"
	"io"
)

// Preflight before typed allocation. Each valid batch has >128 bytes of digest
// text, so this array bound cannot exclude a valid <=1MiB v1 record. Other valid
// arrays are much smaller. It also bounds malicious compact arrays of nulls.
func boundedRecordJSON(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if !recordJSONValue(d, 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

func recordJSONValue(d *json.Decoder, depth int) bool {
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
	switch delim {
	case '{':
		for d.More() {
			if _, err := d.Token(); err != nil {
				return false
			}
			if !recordJSONValue(d, depth+1) {
				return false
			}
		}
	case '[':
		count := 0
		for d.More() {
			count++
			if count > maxRecordBytes/128 || !recordJSONValue(d, depth+1) {
				return false
			}
		}
	default:
		return false
	}
	_, err = d.Token() // Decoder checks the corresponding closing delimiter.
	return err == nil
}
