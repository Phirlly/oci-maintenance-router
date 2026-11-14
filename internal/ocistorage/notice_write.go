package ocistorage

import (
	"context"
	"strings"

	"oci-maintenance-router/internal/noticeledger"
)

func (s *NoticeStore) write(ctx context.Context, id, previousTag string, record noticeledger.Record) (StoredNotice, error) {
	ctx, cancel, err := noticeContext(ctx)
	if err != nil {
		return StoredNotice{}, err
	}
	defer cancel()
	body, err := record.Encode()
	if err != nil {
		return StoredNotice{}, err
	}
	if ctx.Err() != nil {
		return StoredNotice{}, failure(ctx, Canceled, 0, false)
	}
	var tag string
	if previousTag == "" {
		tag, err = s.objects.Create(ctx, noticeObjectName(id), body)
	} else {
		tag, err = s.objects.Replace(ctx, noticeObjectName(id), previousTag, body)
	}
	if err != nil {
		return StoredNotice{}, noticeBackendFailure(ctx, err, true)
	}
	if ctx.Err() != nil {
		return StoredNotice{}, failure(ctx, Canceled, 200, true)
	}
	if !validETag(tag) {
		return StoredNotice{}, &Error{Code: InvalidResponse, Status: 200, WriteUncertain: true}
	}
	return StoredNotice{s, record, strings.Clone(tag)}, nil
}
