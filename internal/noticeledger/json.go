package noticeledger

import (
	"bytes"
	"encoding/json"
	"io"
)

// Bound allocation before typed decoding. Each valid revision/batch consumes
// far more than128 bytes; resource membership is independently capped at256.
func boundedJSON(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if !jsonValue(d, 0, "") {
		return false
	}
	_, e := d.Token()
	return e == io.EOF
}
func jsonValue(d *json.Decoder, depth int, field string) bool {
	t, e := d.Token()
	if e != nil {
		return false
	}
	delim, container := t.(json.Delim)
	if !container {
		return true
	}
	if depth >= 32 {
		return false
	}
	switch delim {
	case '{':
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return false
			}
			name, ok := key.(string)
			if !ok || !jsonValue(d, depth+1, name) {
				return false
			}
		}
	case '[':
		limit := recordLimit / 128
		if field == "resources" {
			limit = 256
		}
		count := 0
		for d.More() {
			count++
			if count > limit || !jsonValue(d, depth+1, "") {
				return false
			}
		}
	default:
		return false
	}
	_, e = d.Token()
	return e == nil
}
