package ocistorage

import (
	"context"
	"strings"
)

// NoticePage is one names-only observation, not a complete or durable snapshot.
// Follow NextStartWith even with no names; the coordinator owns cycle detection,
// total budgets and visible handling of every failed page or record.
type NoticePage struct {
	Names         []string
	NextStartWith string
}

// ListPage lists current notice objects only. It neither reads object versions
// nor authorizes recovery or notification delivery.
func (s *NoticeStore) ListPage(ctx context.Context, start string) (NoticePage, error) {
	if !s.usable() || (start != "" && !validNoticeObjectName(start)) {
		return NoticePage{}, &Error{Code: InvalidRequest}
	}
	ctx, cancel, err := noticeContext(ctx)
	if err != nil {
		return NoticePage{}, err
	}
	defer cancel()
	page, err := s.objects.ListNoticePage(ctx, start)
	if err != nil {
		return NoticePage{}, noticeBackendFailure(ctx, err, false)
	}
	if ctx.Err() != nil {
		return NoticePage{}, failure(ctx, Canceled, 200, false)
	}
	if err := validateNoticePage(page); err != nil {
		return NoticePage{}, failure(ctx, err.Code, 200, false)
	}
	owned := NoticePage{Names: make([]string, len(page.Names)), NextStartWith: strings.Clone(page.NextStartWith)}
	for i, name := range page.Names {
		owned.Names[i] = strings.Clone(name)
	}
	if ctx.Err() != nil {
		return NoticePage{}, failure(ctx, Canceled, 200, false)
	}
	return owned, nil
}
