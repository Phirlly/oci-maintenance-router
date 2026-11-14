package ocinotifications

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

func (c *Client) lifetime(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil {
		return nil, nil, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return nil, nil, failure(ctx, Canceled, 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &Error{Code: DeadlineRequired}
	}
	bounded, cancel := context.WithTimeout(ctx, operationLimit)
	return bounded, cancel, nil
}

// The caller owns the lifetime and, for POST, an immutable byte copy. Each call
// uses one fresh HTTP1 exchange. Status survives response errors; bodies do not.
func (c *Client) exchange(ctx context.Context, method, target string, body []byte) ([]byte, int, error) {
	if ctx.Err() != nil {
		return nil, 0, failure(ctx, Canceled, 0, false)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, 0, &Error{Code: InvalidRequest}
	}
	req.Close = true
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	// A synchronous provider cannot be preempted. Never abandon it or dispatch late.
	if err = c.signer.Sign(req); err != nil {
		return nil, 0, failure(ctx, Authentication, 0, false)
	}
	req.GetBody = nil
	if ctx.Err() != nil {
		return nil, 0, failure(ctx, Canceled, 0, false)
	}
	transport, closeConnections := c.operationTransport(ctx)
	defer closeConnections()
	publish := method == http.MethodPost
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, 0, failure(ctx, Transport, 0, publish)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, failure(ctx, Service, resp.StatusCode, publish)
	}
	data, err := readBody(ctx, resp, publish)
	return data, resp.StatusCode, err
}
