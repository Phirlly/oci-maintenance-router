package ocistorage

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HistoryIsEmpty reports complete retained-version prefix absence, not authority
// to initialize state. Every failure returns false; the caller owns recovery.
func (c *Client) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil || !safeName(name) {
		return false, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return false, failure(ctx, Canceled, 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return false, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, operationLimit)
	defer cancel()
	query := url.Values{"prefix": {name}, "limit": {"1"}}
	path := strings.TrimSuffix(c.path, "/o/") + "/objectversions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path+"?"+query.Encode(), nil)
	if err != nil {
		return false, &Error{Code: InvalidRequest}
	}
	req.Close = true
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Accept", "application/json")
	if err = c.signer.Sign(req); err != nil {
		return false, failure(ctx, Authentication, 0, false)
	}
	if ctx.Err() != nil {
		return false, failure(ctx, Canceled, 0, false)
	}
	client, closeConnections := c.operationHTTP(ctx)
	defer closeConnections()
	resp, err := client.Transport.RoundTrip(req)
	if err != nil {
		return false, failure(ctx, Transport, 0, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, failure(ctx, Service, resp.StatusCode, false)
	}
	// Even an empty or repeated continuation header makes the result unusable.
	if len(resp.Header.Values("opc-next-page")) != 0 {
		return false, failure(ctx, InvalidResponse, 200, false)
	}
	body, err := readListingBody(ctx, resp)
	if err != nil {
		return false, err
	}
	// Go keeps declared and late trailers separate from Header. Presence alone
	// is ambiguous, including an advertised key whose value remains empty.
	if _, continued := resp.Trailer["Opc-Next-Page"]; continued {
		return false, failure(ctx, InvalidResponse, 200, false)
	}
	empty, err := decodeHistoryEmpty(body, name)
	if ctx.Err() != nil {
		return false, failure(ctx, Canceled, 200, false)
	}
	if err != nil {
		code := InvalidResponse
		if detail, ok := err.(*Error); ok {
			code = detail.Code
		}
		return false, failure(ctx, code, 200, false)
	}
	return empty, nil
}
