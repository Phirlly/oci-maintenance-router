package ocistorage

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// This conservative application policy accepts opaque bare or quoted tags, not
// weak validators, wildcard/list conditions or arbitrary header text.
func validETag(tag string) bool {
	if len(tag) == 0 || len(tag) > 1024 || strings.HasPrefix(tag, "W/") {
		return false
	}
	if tag[0] == '"' && tag[len(tag)-1] == '"' && len(tag) >= 2 {
		tag = tag[1 : len(tag)-1]
	}
	if tag == "" {
		return false
	}
	for _, c := range tag {
		if c <= 32 || c >= 127 || strings.ContainsRune("\"\\,*", c) {
			return false
		}
	}
	return true
}

func readObject(ctx context.Context, resp *http.Response, write bool) (Object, error) {
	tags := resp.Header.Values("ETag")
	if len(tags) != 1 || !validETag(tags[0]) || len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return Object{}, failure(ctx, InvalidResponse, resp.StatusCode, write)
	}
	limit := objectLimit
	if write {
		limit = 4 << 10
	}
	if resp.ContentLength > int64(limit) {
		return Object{}, failure(ctx, TooLarge, resp.StatusCode, write)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return Object{}, failure(ctx, InvalidResponse, resp.StatusCode, write)
	}
	if len(body) > limit {
		return Object{}, failure(ctx, TooLarge, resp.StatusCode, write)
	}
	if write && len(body) != 0 {
		return Object{}, failure(ctx, InvalidResponse, resp.StatusCode, true)
	}
	if err = resp.Body.Close(); err != nil {
		return Object{}, failure(ctx, InvalidResponse, resp.StatusCode, write)
	}
	if ctx.Err() != nil {
		return Object{}, failure(ctx, Canceled, resp.StatusCode, write)
	}
	return Object{Body: body, ETag: tags[0]}, nil
}
