package reconciliation

import (
	"context"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// AnnouncementSource must validate fixed-tenancy pages and requested detail
// identity. Runtime composition owns association with the collector's config.
type AnnouncementSource interface {
	AnnouncementPages
	Get(context.Context, string) (notices.Notice, error)
}

// SavedNoticeSource must bind each canonical record to the requested private
// object path and environment. NoticeStore provides that complete validation.
type SavedNoticeSource interface {
	SavedNoticePages
	LoadRecord(context.Context, string) (noticeledger.Record, error)
}

// SavedNoticeRef identifies observed saved work, including saved-only notices.
// Later processing must reread it and check current source; no ETag is retained.
type SavedNoticeRef struct{ ObjectName, NoticeID string }

// Collection contains observed occurrences, not an authoritative snapshot,
// baseline, heartbeat, complete chain hold decision or permission to send.
type Collection struct {
	Current []CandidateMatch
	Saved   []SavedNoticeRef
}

// Collector owns immutable policy and trusted adapters. Invocations use fresh
// state and may run concurrently if the supplied adapters support concurrent use.
type Collector struct {
	announcements AnnouncementSource
	saved         SavedNoticeSource
	policy        *routing.Policy
	environmentID string
}

func NewCollector(data []byte, catalog configuration.Catalog, location configuration.LedgerLocation, announcements AnnouncementSource, saved SavedNoticeSource) (*Collector, error) {
	if announcements == nil || saved == nil {
		return nil, &CollectionError{Code: InvalidRequest}
	}
	facts, err := NewCandidates(data, catalog, location)
	if err != nil {
		return nil, err
	}
	return &Collector{announcements: announcements, saved: saved, policy: facts.policy, environmentID: facts.environmentID}, nil
}

// Collect lists both sources and reads every listed occurrence sequentially
// under one shared deadline and pacing state. Failure returns no partial result.
// It retains compact facts only; later processing must reread and compare full
// revisions before acting. No state writes, retries or downstream calls occur.
func (c *Collector) Collect(ctx context.Context) (Collection, error) { return c.collect(ctx, pause) }

func (c *Collector) collect(ctx context.Context, wait func(context.Context, time.Duration) error) (Collection, error) {
	if c == nil || c.policy == nil || c.environmentID == "" || c.announcements == nil || c.saved == nil {
		return Collection{}, &CollectionError{Code: InvalidRequest}
	}
	ctx, cancel, startErr := discoveryContext(ctx)
	if startErr != nil {
		return Collection{}, (*CollectionError)(startErr)
	}
	defer cancel()
	e := enumeration{ctx: ctx, wait: wait}
	inventory, err := e.inventory(c.announcements, c.saved)
	if err != nil {
		return Collection{}, collectionFailure(ctx, "", err)
	}
	facts := &Candidates{policy: c.policy, environmentID: c.environmentID}
	if err = e.currentDetails(c.announcements, inventory.AnnouncementIDs, facts); err != nil {
		return Collection{}, err
	}
	saved, err := e.savedDetails(c.saved, inventory.NoticeObjects, facts)
	if err != nil {
		return Collection{}, err
	}
	if ctx.Err() != nil {
		return Collection{}, (*CollectionError)(failure(ctx, "", Canceled))
	}
	current, err := facts.Resolve()
	if err != nil {
		return Collection{}, collectionFailure(ctx, "", err)
	}
	if ctx.Err() != nil {
		return Collection{}, (*CollectionError)(failure(ctx, "", Canceled))
	}
	return Collection{Current: current, Saved: saved}, nil
}
