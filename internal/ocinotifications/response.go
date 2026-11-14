package ocinotifications

import (
	"context"
	"io"
	"net/http"
)

func readBody(ctx context.Context, resp *http.Response, sent bool) ([]byte, error) {
	if len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, sent)
	}
	if resp.ContentLength > bodyLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode, sent)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, sent)
	}
	if len(body) > bodyLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode, sent)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode, sent)
	}
	if ctx.Err() != nil {
		return nil, failure(ctx, Canceled, resp.StatusCode, sent)
	}
	return body, nil
}
