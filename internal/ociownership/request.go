package ociownership

import (
	"context"
	"net/http"
	"time"

	"oci-maintenance-router/internal/routing"
)

// Get makes one unconditional read and returns an owned metadata projection.
// Success proves neither allowed scope nor send authority. Errors, including404,
// require caller-visible unresolved handling, never an inference of deletion.
func (c *Client) Get(ctx context.Context, id string) (routing.Metadata, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil || !c.validResourceID(id) {
		return routing.Metadata{}, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return routing.Metadata{}, failure(ctx, Canceled, 0)
	}
	if _, ok := ctx.Deadline(); !ok {
		return routing.Metadata{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+c.resourcePath+id, nil)
	if err != nil {
		return routing.Metadata{}, &Error{Code: InvalidRequest}
	}
	req.Close = true // Prevent post-close draining after early rejection.
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	// Signing is synchronous. Do not abandon a blocked provider in a goroutine,
	// or dispatch after it exhausts the caller's remaining budget.
	if err = c.signer.Sign(req); err != nil {
		return routing.Metadata{}, failure(ctx, Authentication, 0)
	}
	if ctx.Err() != nil {
		return routing.Metadata{}, failure(ctx, Canceled, 0)
	}
	transport, closeConnections := c.operationTransport(ctx)
	defer closeConnections()
	resp, err := transport.RoundTrip(req) // No redirect parsing/following or retries.
	if err != nil {
		return routing.Metadata{}, failure(ctx, Transport, 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return routing.Metadata{}, failure(ctx, Service, resp.StatusCode)
	}
	body, err := readBody(ctx, resp)
	if err != nil {
		return routing.Metadata{}, err
	}
	metadata, err := decodeMetadata(body, id, c.tenancy)
	if ctx.Err() != nil {
		return routing.Metadata{}, failure(ctx, Canceled, 200)
	}
	if err != nil {
		code := InvalidResponse
		if e, ok := err.(*Error); ok {
			code = e.Code
		}
		return routing.Metadata{}, failure(ctx, code, 200)
	}
	return metadata, nil
}
