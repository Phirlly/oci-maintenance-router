package ociannouncements

import (
	"context"
	"net/url"

	"oci-maintenance-router/internal/notices"
)

// Get returns a fully validated detail or no notice. A failed read is not
// evidence of deletion or unrelatedness; the caller must retain unresolved work.
func (c *Client) Get(ctx context.Context, id string) (notices.Notice, error) {
	if !validID(id, announcementID) {
		return notices.Notice{}, &Error{Code: InvalidRequest}
	}
	ctx, cancel, err := c.start(ctx)
	if err != nil {
		return notices.Notice{}, err
	}
	defer cancel()
	body, _, err := c.read(ctx, "/20180904/announcements/"+id, nil)
	if err != nil {
		return notices.Notice{}, err
	}
	notice, err := decodeNotice(body, id)
	if err = decodedError(ctx, err); err != nil {
		return notices.Notice{}, err
	}
	return notice, nil
}

// ListPage reads exactly one unfiltered, all-chain page. See Page for the
// caller's mandatory pagination, detail-fetch and completeness responsibilities.
func (c *Client) ListPage(ctx context.Context, page string) (Page, error) {
	if !validPageToken(page) {
		return Page{}, &Error{Code: InvalidRequest}
	}
	ctx, cancel, err := c.start(ctx)
	if err != nil {
		return Page{}, err
	}
	defer cancel()
	query := url.Values{"compartmentId": {c.tenancy}, "limit": {"100"}, "shouldShowOnlyLatestInChain": {"false"}}
	if page != "" {
		query.Set("page", page)
	}
	body, header, err := c.read(ctx, "/20180904/announcements", query)
	if err != nil {
		return Page{}, err
	}
	result, err := decodePage(body, header, page)
	if err = decodedError(ctx, err); err != nil {
		return Page{}, err
	}
	return result, nil
}

func (c *Client) start(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if c == nil || c.signer == nil || c.transport == nil || ctx == nil {
		return nil, nil, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return nil, nil, failure(ctx, Canceled, 0)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &Error{Code: DeadlineRequired}
	}
	operation, cancel := context.WithTimeout(ctx, operationLimit)
	return operation, cancel, nil
}

func decodedError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return failure(ctx, Canceled, 200)
	}
	if err != nil {
		code := InvalidResponse
		if e, ok := err.(*Error); ok {
			code = e.Code
		}
		return failure(ctx, code, 200)
	}
	return nil
}
