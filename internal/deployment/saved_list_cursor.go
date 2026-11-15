package deployment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"regexp"
)

// A consistency token, not authority. It identifies a service page and, for
// subpages, binds its observed membership. No whole-inventory snapshot is held.
type listingCursor struct {
	Version    int    `json:"version"`
	Context    string `json:"context"`
	Start      string `json:"start"`
	Page       int    `json:"page"`
	Offset     int    `json:"offset"`
	PageDigest string `json:"pageDigest,omitempty"`
}

var listingStartPattern = regexp.MustCompile(`^notices/[0-9a-f]{64}\.json$`)

func encodeListingCursor(c listingCursor) string {
	body, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(body)
}

func parseListingCursor(text string) (listingCursor, error) {
	var c listingCursor
	if text == "" {
		return c, nil
	}
	invalid := operationFailure("listing_cursor_invalid")
	if len(text) > 512 {
		return c, invalid
	}
	body, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || base64.RawURLEncoding.EncodeToString(body) != text || json.Unmarshal(body, &c) != nil {
		return listingCursor{}, invalid
	}
	canonical, _ := json.Marshal(c)
	// Equality also rejects duplicate, unknown, omitted and case-aliased fields,
	// nulls, fractional numbers, whitespace and trailing JSON.
	if !bytes.Equal(body, canonical) || c.Version != 1 || !hashPattern.MatchString(c.Context) ||
		c.Page < 0 || c.Page >= 100 || c.Offset < 0 || c.Offset >= 1000 ||
		(c.Page == 0) != (c.Start == "") || c.Start != "" && !listingStartPattern.MatchString(c.Start) ||
		c.Offset == 0 && c.PageDigest != "" || c.Offset > 0 && !hashPattern.MatchString(c.PageDigest) {
		return listingCursor{}, invalid
	}
	return c, nil
}
