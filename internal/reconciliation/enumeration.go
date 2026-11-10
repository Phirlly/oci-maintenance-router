// Package reconciliation coordinates bounded discovery work. Enumeration alone
// establishes neither a complete source scan nor permission to deliver notices.
package reconciliation

import (
	"context"
	"time"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

// AnnouncementPages must be a trusted, fixed-scope adapter that validates every
// page. Bootstrap owns its tenancy association; the OCI client satisfies this.
type AnnouncementPages interface {
	ListPage(context.Context, string) (ociannouncements.Page, error)
}

// SavedNoticePages must validate current private notice paths for one verified
// storage location. The existing NoticeStore satisfies this interface.
type SavedNoticePages interface {
	ListPage(context.Context, string) (ocistorage.NoticePage, error)
}

// Inventory owns identifiers in observed order, including duplicate occurrences.
// It contains no details or saved records. It is not a snapshot, baseline,
// successful-scan heartbeat, or authorization to process or send saved content.
type Inventory struct {
	AnnouncementIDs []string
	NoticeObjects   []string
}

// Enumerate follows both listings from their beginning under a shared caller
// deadline capped at 120s. Every error returns no inventory. There is no retry,
// resume, deduplication, filtering, persistence or delivery in this operation.
func Enumerate(ctx context.Context, announcements AnnouncementPages, saved SavedNoticePages) (Inventory, error) {
	return enumerate(ctx, announcements, saved, pause)
}

func enumerate(ctx context.Context, announcements AnnouncementPages, saved SavedNoticePages, wait func(context.Context, time.Duration) error) (Inventory, error) {
	if ctx == nil || announcements == nil || saved == nil {
		return Inventory{}, &Error{Code: InvalidRequest}
	}
	if ctx.Err() != nil {
		return Inventory{}, failure(ctx, "", Canceled)
	}
	if _, ok := ctx.Deadline(); !ok {
		return Inventory{}, &Error{Code: DeadlineRequired}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	e := enumeration{ctx: ctx, wait: wait}
	ids, err := e.announcements(announcements)
	if err != nil {
		return Inventory{}, err
	}
	names, err := e.savedNotices(saved)
	if err != nil {
		return Inventory{}, err
	}
	if ctx.Err() != nil {
		return Inventory{}, failure(ctx, SavedNotices, Canceled)
	}
	return Inventory{ids, names}, nil
}

func pause(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
