package pagerduty

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

// Send makes one attempt using the first saved envelope and its pinned key.
// The coordinator must select this client's saved region, read the saved secret
// ID AND version, persist SENDING, and enforce source/lease/activation gates.
// Validation here does not prove provenance, key ownership or permission to send.
// Callers must not mutate inputs concurrently with Send; it owns the wire body
// before dispatch. No success body is required or exposed.
func (c *Client) Send(ctx context.Context, savedEnvelope, routingKey []byte) (Response, error) {
	if c == nil || c.transport == nil || ctx == nil {
		return Response{}, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return Response{}, failure(ctx, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return Response{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	body, err := envelope(savedEnvelope, routingKey)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, &Error{Code: InvalidRequest}
	}
	req.GetBody = nil
	// DisableKeepAlives only adds a wire header. Request.Close also prevents
	// Go's asynchronous post-close body draining when a peer omits that header.
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if ctx.Err() != nil {
		return Response{}, failure(ctx, false)
	}
	transport, closeConnections := c.operationTransport(ctx)
	defer closeConnections()
	// RoundTrip preserves all observed statuses without parsing redirect URLs.
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return Response{}, failure(ctx, true)
	}
	received := time.Now().UTC()
	// Preserve an observed status even if the body is broken or cancellation
	// follows headers. Turning an accepted response into uncertainty risks replay.
	_ = resp.Body.Close()
	result := Response{Status: resp.StatusCode}
	if result.Status == 429 || (result.Status >= 500 && result.Status <= 599) {
		result.RetryAfter, result.RetryAfterOverflow = retryAfter(resp.Header, received)
	}
	return result, nil
}
