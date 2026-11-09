package ocistorage

import (
	"context"
	"io"
	"net/http"
)

// Listings carry no required ETag. They never supply a conditional write token.
func readListingBody(ctx context.Context, resp *http.Response) ([]byte, error) {
	if len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, false)
	}
	if resp.ContentLength > objectLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode, false)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, objectLimit+1))
	if err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, false)
	}
	if len(body) > objectLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode, false)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, false)
	}
	if ctx.Err() != nil {
		return nil, failure(ctx, Canceled, resp.StatusCode, false)
	}
	return body, nil
}
