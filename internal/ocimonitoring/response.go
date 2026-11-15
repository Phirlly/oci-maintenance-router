package ocimonitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"
)

const bodyLimit = 16 << 10

func acceptedResponse(ctx context.Context, resp *http.Response) error {
	if len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 || resp.ContentLength > bodyLimit {
		return failure(ctx, "invalid_response")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		return failure(ctx, "invalid_response")
	}
	if err = resp.Body.Close(); err != nil {
		return failure(ctx, "invalid_response")
	}
	if ctx.Err() != nil {
		return failure(ctx, "canceled")
	}
	if !validAcceptance(body) {
		return failure(ctx, "invalid_response")
	}
	return nil
}

// The success shape has only two fields and no nested data. Parse their names
// explicitly to reject duplicate (including escaped) keys and missing counts.
func validAcceptance(body []byte) bool {
	if !utf8.Valid(body) || !json.Valid(body) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	token, _ := d.Token()
	if token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		key, _ := d.Token()
		name, ok := key.(string)
		if !ok || seen[name] {
			return false
		}
		seen[name] = true
		switch name {
		case "failedMetricsCount":
			value, _ := d.Token()
			number, ok := value.(json.Number)
			if !ok {
				return false
			}
			count, err := number.Int64()
			if err != nil || count != 0 {
				return false
			}
		case "failedMetrics":
			var entries []json.RawMessage
			if d.Decode(&entries) != nil || len(entries) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return seen["failedMetricsCount"]
}
