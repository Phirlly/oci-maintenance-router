package ocistorage

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ListNoticePage lists one current notices/ page using the fixed object endpoint.
// The caller owns traversal, durable interpretation and recovery authority.
func (c *Client) ListNoticePage(ctx context.Context, start string) (NoticePage, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil ||
		(start != "" && !validNoticeObjectName(start)) {
		return NoticePage{}, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return NoticePage{}, failure(ctx, Canceled, 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return NoticePage{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	query := url.Values{"prefix": {"notices/"}, "limit": {"1000"}}
	if start != "" {
		query.Set("start", start)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+strings.TrimSuffix(c.path, "/")+"?"+query.Encode(), nil)
	if err != nil {
		return NoticePage{}, &Error{Code: InvalidRequest}
	}
	req.Close = true
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	if err = c.signer.Sign(req); err != nil {
		return NoticePage{}, failure(ctx, Authentication, 0, false)
	}
	if ctx.Err() != nil {
		return NoticePage{}, failure(ctx, Canceled, 0, false)
	}
	client, closeConnections := c.operationHTTP(ctx)
	defer closeConnections()
	resp, err := client.Transport.RoundTrip(req)
	if err != nil {
		return NoticePage{}, failure(ctx, Transport, 0, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NoticePage{}, failure(ctx, Service, resp.StatusCode, false)
	}
	body, err := readListingBody(ctx, resp)
	if err != nil {
		return NoticePage{}, err
	}
	page, err := decodeNoticePage(body)
	if ctx.Err() != nil {
		return NoticePage{}, failure(ctx, Canceled, 200, false)
	}
	if err != nil {
		code := InvalidResponse
		if e, ok := err.(*Error); ok {
			code = e.Code
		}
		return NoticePage{}, failure(ctx, code, 200, false)
	}
	return page, nil
}
