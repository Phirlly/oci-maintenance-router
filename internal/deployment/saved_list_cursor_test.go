package deployment

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestListingCursorAcceptsCanonicalBoundaries(t *testing.T) {
	for _, c := range []listingCursor{
		{Version: 1, Context: strings.Repeat("a", 64)},
		{Version: 1, Context: strings.Repeat("a", 64), Start: "notices/" + strings.Repeat("b", 64) + ".json", Page: 99, Offset: 999, PageDigest: strings.Repeat("c", 64)},
	} {
		cursor := encodeListingCursor(c)
		got, err := parseListingCursor(cursor)
		if err != nil || got != c || len(cursor) > 512 {
			t.Fatalf("invalid roundtrip: %+v %v", got, err)
		}
		if _, err := parseCommand([]byte(listInvocation(cursor))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListingCursorRejectsMalformedAndNoncanonicalInput(t *testing.T) {
	base := listingCursor{Version: 1, Context: strings.Repeat("a", 64)}
	valid, _ := json.Marshal(base)
	for _, text := range []string{
		string(valid) + " ", string(valid) + `{}`, `{}`, `null`,
		strings.Replace(string(valid), `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(string(valid), `"version":1`, `"version":"1"`, 1),
		strings.Replace(string(valid), `"version":1`, `"unknown":1,"version":1`, 1),
		strings.Replace(string(valid), `"offset":0`, `"offset":null`, 1),
	} {
		if _, err := parseListingCursor(base64.RawURLEncoding.EncodeToString([]byte(text))); err == nil {
			t.Fatalf("noncanonical cursor accepted: %q", text)
		}
	}
	for _, edit := range []func(*listingCursor){
		func(c *listingCursor) { c.Version = 2 },
		func(c *listingCursor) { c.Context = "bad" },
		func(c *listingCursor) { c.Start = "private/path" },
		func(c *listingCursor) { c.Start = "notices/" + strings.Repeat("b", 64) + ".json" },
		func(c *listingCursor) { c.Page = 1 },
		func(c *listingCursor) { c.Page = -1 },
		func(c *listingCursor) { c.Page = 100 },
		func(c *listingCursor) { c.Offset = -1 },
		func(c *listingCursor) { c.Offset = 1000 },
		func(c *listingCursor) { c.Offset = 1 },
		func(c *listingCursor) { c.PageDigest = strings.Repeat("c", 64) },
		func(c *listingCursor) { c.Offset = 1; c.PageDigest = "bad" },
	} {
		c := base
		edit(&c)
		if _, err := parseListingCursor(encodeListingCursor(c)); err == nil {
			t.Fatalf("invalid cursor accepted: %+v", c)
		}
	}
	for _, cursor := range []string{"!", strings.Repeat("a", 513), encodeListingCursor(base) + "=", encodeListingCursor(base) + "\n"} {
		if _, err := parseListingCursor(cursor); err == nil {
			t.Fatalf("invalid encoding accepted: %q", cursor)
		}
	}
}
