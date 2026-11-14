package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

func (r *processingRun) fresh(work processingNoticeWork, saved ocistorage.StoredNotice) (ocistorage.StoredNotice, notices.Notice, map[string]routing.Lookup, bool, error) {
	n, err := r.readSource(work.id)
	if err != nil {
		if status, ok := unavailableStatus(err); ok {
			next, transition := saved.Record().RecordSourceUnavailable(status, r.lease.manager.now())
			if transition != nil {
				return saved, n, nil, false, &ProcessingError{Code: "transition_failed"}
			}
			saved, transition = r.save(saved, next)
			if transition == nil {
				transition = r.issue(ProcessingIssue{NoticeID: work.id, Code: "source_unavailable"})
			}
			return saved, n, nil, false, transition
		}
		// readSource already admits only classified unavailability or its own
		// controlled failure; provider errors cannot cross that boundary.
		return saved, n, nil, false, err
	}
	chain := ""
	if value := n.Snapshot().ChainID; value != nil {
		chain = *value
	}
	if n.Key() != work.member.Revision || chain != work.member.ChainID {
		return saved, n, nil, false, r.changedSource(work, saved, n, chain)
	}
	_, targets := r.owner.policy.Targets(n, true)
	lookups := map[string]routing.Lookup{}
	for _, target := range targets {
		if err = r.before(); err != nil {
			return saved, n, nil, false, err
		}
		metadata, failure := r.owner.mysql[target.Region](r.ctx, target.ResourceID)
		if r.ctx.Err() != nil {
			return saved, n, nil, false, processingFailure(r.ctx, nil)
		}
		lookup := routing.Lookup{State: routing.Found, Metadata: metadata}
		if failure != nil {
			lookup = routing.Lookup{State: routing.Unavailable}
			if e, ok := failure.(*ocimysql.Error); ok && e != nil && e.Code == ocimysql.Service {
				if e.Status == 401 || e.Status == 403 {
					lookup.State = routing.Denied
				}
				if e.Status == 404 {
					lookup.State = routing.NotFound
				}
			}
		}
		lookups[target.ResourceID] = lookup
	}
	next, err := r.owner.observer.Observe(saved.Record(), n, true, lookups, r.lease.manager.now())
	if err != nil {
		return saved, n, nil, false, &ProcessingError{Code: "observation_failed"}
	}
	saved, err = r.save(saved, next)
	return saved, n, lookups, err == nil, err
}

func (r *processingRun) readSource(id string) (notices.Notice, error) {
	if err := r.guard(); err != nil {
		return notices.Notice{}, processingFailure(r.ctx, err)
	}
	if err := r.before(); err != nil {
		return notices.Notice{}, processingFailure(r.ctx, err)
	}
	n, err := r.owner.discovery.collector.announcements.Get(r.ctx, id)
	if r.ctx.Err() != nil {
		return notices.Notice{}, processingFailure(r.ctx, nil)
	}
	if err != nil {
		if n.Key().NoticeID != "" {
			return notices.Notice{}, &ProcessingError{Code: "source_invalid"}
		}
		if _, ok := unavailableStatus(err); ok {
			return notices.Notice{}, err
		}
		return notices.Notice{}, &ProcessingError{Code: "source_read_failed"}
	}
	if n.Key().NoticeID != id {
		return notices.Notice{}, &ProcessingError{Code: "source_invalid"}
	}
	return n, nil
}

func (r *processingRun) changedSource(work processingNoticeWork, saved ocistorage.StoredNotice, n notices.Notice, chain string) error {
	next, err := r.owner.observer.Observe(saved.Record(), n, true, nil, r.lease.manager.now())
	if err != nil {
		return &ProcessingError{Code: "observation_failed"}
	}
	if _, err = r.save(saved, next); err != nil {
		return err
	}
	seen := map[string]bool{work.id: true}
	for _, component := range r.final.view.components {
		for _, m := range component.Members {
			id := m.Revision.NoticeID
			if seen[id] || chain == "" || m.ChainID != chain {
				continue
			}
			seen[id] = true
			if err = r.guard(); err != nil {
				return err
			}
			other, readErr := r.store.Load(r.ctx, id)
			if readErr != nil {
				return readErr
			}
			if other.Record().Snapshot().SchemaVersion == 3 {
				return &ProcessingError{Code: "unsupported_notice_schema"}
			}
			held, holdErr := other.Record().Hold(delivery.ReasonSourceChanged, r.lease.manager.now())
			if holdErr != nil {
				return &ProcessingError{Code: "transition_failed"}
			}
			if _, err = r.save(other, held); err != nil {
				return err
			}
		}
	}
	return &ProcessingError{Code: "source_changed"}
}
