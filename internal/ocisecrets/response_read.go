package ocisecrets

import (
	"context"
	"io"
	"net/http"
)

// Separate wire budget leaves room for Base64 and optional service metadata.
const responseLimit = 256 << 10

func readBundle(ctx context.Context, resp *http.Response, secretID string, version int64) (Bundle, error) {
	if len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return Bundle{}, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if resp.ContentLength > responseLimit {
		return Bundle{}, failure(ctx, TooLarge, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		return Bundle{}, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if len(body) > responseLimit {
		return Bundle{}, failure(ctx, TooLarge, resp.StatusCode)
	}
	if err = resp.Body.Close(); err != nil {
		return Bundle{}, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if ctx.Err() != nil {
		return Bundle{}, failure(ctx, Canceled, resp.StatusCode)
	}
	bundle, err := decodeBundle(body, secretID, version)
	if ctx.Err() != nil {
		return Bundle{}, failure(ctx, Canceled, resp.StatusCode)
	}
	return bundle, err
}
