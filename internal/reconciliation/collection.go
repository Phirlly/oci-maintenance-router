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
	// Availability contains only classified saved-only failures, not durable state.
	Availability SourceAvailability
	// Candidates includes relevant current and saved historical membership.
	Candidates CandidateView
	// Observations retains original candidate occurrences in Current order,
	// omitting unrelated entries. Applying one still requires conditional saving.
	Observations []noticeledger.Observation
	// Current includes successful listed occurrences followed by successful
	// unique saved-only source reads in first saved-occurrence order.
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
	observer      *noticeledger.Observer
}

func NewCollector(data []byte, catalog configuration.Catalog, location configuration.LedgerLocation, announcements AnnouncementSource, saved SavedNoticeSource) (*Collector, error) {
	if announcements == nil || saved == nil {
		return nil, &CollectionError{Code: InvalidRequest}
	}
	facts, err := NewCandidates(data, catalog, location)
	if err != nil {
		return nil, err
	}
	observer, err := noticeledger.New(data, catalog, location)
	if err != nil {
		return nil, err
	}
	return &Collector{announcements: announcements, saved: saved, policy: facts.policy, environmentID: facts.environmentID, observer: observer}, nil
}

// Collect lists both sources, reads every listed occurrence and checks unique
// saved notices absent from the current list under one deadline and pacing state.
// Classified saved-only failures return availability evidence; fatal errors
// return no partial result.
// It retains redacted candidate observations; later processing must persist them
// and check current revisions before acting. No writes or downstream calls occur.
func (c *Collector) Collect(ctx context.Context) (Collection, error) { return c.collect(ctx, pause) }

func (c *Collector) collect(ctx context.Context, wait func(context.Context, time.Duration) error) (Collection, error) {
	if c == nil || c.policy == nil || c.environmentID == "" || c.observer == nil || c.announcements == nil || c.saved == nil {
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
	observations := &sourceObservations{facts: facts, observer: c.observer, remaining: observationByteLimit}
	if err := e.currentDetails(c.announcements, inventory.AnnouncementIDs, observations); err != nil {
		return Collection{}, err
	}
	saved, err := e.savedDetails(c.saved, inventory.NoticeObjects, facts)
	if err != nil {
		return Collection{}, err
	}
	unavailable, err := e.savedSources(c.announcements, inventory.AnnouncementIDs, saved, observations)
	if err != nil {
		return Collection{}, err
	}
	if ctx.Err() != nil {
		return Collection{}, (*CollectionError)(failure(ctx, "", Canceled))
	}
	view, err := facts.ResolveView()
	if err != nil {
		return Collection{}, collectionFailure(ctx, "", err)
	}
	current := view.Current()
	// Allocate separately so discarded captures are not retained through an
	// unused backing-array tail. Current and observations have identical order.
	var relevant []noticeledger.Observation
	for i, match := range current {
		if match.KnownCandidate {
			relevant = append(relevant, observations.values[i])
		}
	}
	if ctx.Err() != nil {
		return Collection{}, (*CollectionError)(failure(ctx, "", Canceled))
	}
	return Collection{Current: current, Saved: saved, Observations: relevant, Candidates: view, Availability: SourceAvailability{environmentID: c.environmentID, unavailable: unavailable}}, nil
}
