package ocistorage

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"slices"
	"time"
)

const objectLimit = 1 << 20

// Object contains independently owned bytes and their observed storage version.
// It is not validated domain state or permission to send notifications.
type Object struct {
	Body []byte
	ETag string
}

func (c *Client) Get(ctx context.Context, name string) (Object, error) {
	return c.exchange(ctx, http.MethodGet, name, "", "", nil)
}

// Create never overwrites an existing object. Conflict requires a fresh read.
func (c *Client) Create(ctx context.Context, name string, body []byte) (string, error) {
	obj, err := c.exchange(ctx, http.MethodPut, name, "If-None-Match", "*", body)
	return obj.ETag, err
}

// Replace requires the exact ETag from a previous successful read or write.
// The caller must reread/redecide after conflict or an uncertain write.
func (c *Client) Replace(ctx context.Context, name, etag string, body []byte) (string, error) {
	if !validETag(etag) {
		return "", &Error{Code: InvalidRequest}
	}
	obj, err := c.exchange(ctx, http.MethodPut, name, "If-Match", etag, body)
	return obj.ETag, err
}

func (c *Client) exchange(ctx context.Context, method, name, condition, etag string, body []byte) (Object, error) {
	if c == nil || c.signer == nil || ctx == nil || !safeName(name) {
		return Object{}, &Error{Code: InvalidRequest}
	}
	if len(body) > objectLimit {
		return Object{}, &Error{Code: TooLarge}
	}
	if ctx.Err() != nil {
		return Object{}, failure(ctx, Canceled, 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return Object{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	// Copy before signing: caller-owned bytes cannot change the signed upload.
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+c.path+url.PathEscape(name), bytes.NewReader(slices.Clone(body)))
	if err != nil {
		return Object{}, &Error{Code: InvalidRequest}
	}
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(condition, etag)
	}
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	// Synchronous credential I/O is not preemptible. Never abandon a signer in
	// a goroutine; check its caller's budget again before any network dispatch.
	if err = c.signer.Sign(req); err != nil {
		return Object{}, failure(ctx, Authentication, 0, false)
	}
	if ctx.Err() != nil {
		return Object{}, failure(ctx, Canceled, 0, false)
	}
	client, closeConnections := c.operationHTTP(ctx)
	defer closeConnections()
	write := method == http.MethodPut
	resp, err := client.Do(req)
	if err != nil {
		return Object{}, failure(ctx, Transport, 0, write)
	}
	defer resp.Body.Close()
	if write && resp.StatusCode == http.StatusPreconditionFailed {
		return Object{}, &Error{Code: Conflict, Status: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		return Object{}, &Error{Code: Service, Status: resp.StatusCode, WriteUncertain: write}
	}
	return readObject(ctx, resp, write)
}
