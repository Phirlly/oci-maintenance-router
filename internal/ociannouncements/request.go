package ociannouncements

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) read(ctx context.Context, path string, query url.Values) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return nil, nil, &Error{Code: InvalidRequest}
	}
	req.URL.RawQuery = query.Encode()
	req.Close = true // Prevent asynchronous body draining after early rejection.
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	// Signing is synchronous: never abandon a blocked provider in a goroutine
	// and never dispatch if signing consumed the operation's remaining budget.
	if err = c.signer.Sign(req); err != nil {
		return nil, nil, failure(ctx, Authentication, 0)
	}
	if ctx.Err() != nil {
		return nil, nil, failure(ctx, Canceled, 0)
	}
	transport, closeConnections := c.operationTransport(ctx)
	defer closeConnections()
	// Direct RoundTrip preserves redirect statuses, even with malformed Location.
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, nil, failure(ctx, Transport, 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, failure(ctx, Service, resp.StatusCode)
	}
	body, err := readBody(ctx, resp)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}
