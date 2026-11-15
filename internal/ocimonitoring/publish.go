package ocimonitoring

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

// Publish sends one immutable observation without replay. Cancellation prevents
// dispatch even after a slow synchronous signer. Inputs must not be mutated
// concurrently; response uncertainty does not authorize another request.
func (c *Client) Publish(ctx context.Context, observation Observation) error {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil {
		return &Error{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return failure(ctx, "canceled")
	}
	if _, ok := ctx.Deadline(); !ok {
		return &Error{Code: "deadline_required"}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	body, err := c.payload(observation, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://telemetry-ingestion."+c.region+".oraclecloud.com/20180401/metrics", bytes.NewReader(body))
	if err != nil {
		return &Error{Code: "invalid_request"}
	}
	defer req.Body.Close()
	req.Close = true
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if err = c.signer.Sign(req); err != nil {
		return failure(ctx, "authentication")
	}
	req.GetBody = nil
	if ctx.Err() != nil {
		return failure(ctx, "canceled")
	}
	transport, closeConnections := c.operationTransport(ctx)
	defer closeConnections()
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return failure(ctx, "transport")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failure(ctx, "service")
	}
	if err = acceptedResponse(ctx, resp); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return failure(ctx, "canceled")
	}
	return nil
}
