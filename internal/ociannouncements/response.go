package ociannouncements

import (
	"context"
	"io"
	"net/http"
)

func readBody(ctx context.Context, resp *http.Response) ([]byte, error) {
	if len(resp.Header.Values("Content-Range")) != 0 || len(resp.Header.Values("Content-Encoding")) != 0 {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if resp.ContentLength > bodyLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if len(body) > bodyLimit {
		return nil, failure(ctx, TooLarge, resp.StatusCode)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, failure(ctx, InvalidResponse, resp.StatusCode)
	}
	if ctx.Err() != nil {
		return nil, failure(ctx, Canceled, resp.StatusCode)
	}
	return body, nil
}
