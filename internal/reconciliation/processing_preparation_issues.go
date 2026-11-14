package reconciliation

import (
	"bytes"
	"context"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

type preparationIssueStorage struct {
	discoveryStorage
	objects PreparationIssueObjects
}

func (s preparationIssueStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}

// recordPreparation persists evidence before continuing preparation or sending.
// The original deadline and CURRENT/lease checks apply to every conditional write
// and exact confirmation; this is never detached outcome persistence.
func (r *processingRun) recordPreparation(saved ocistorage.StoredNotice, n notices.Notice, lookups map[string]routing.Lookup, code string, ids ...string) error {
	indexes := []int{}
	snapshot := saved.Record().Snapshot()
	for _, id := range ids {
		found := false
		for _, revision := range snapshot.Revisions {
			if revision.Digest != n.Key().Digest {
				continue
			}
			for _, row := range revision.Resources {
				if row.ResourceID == id {
					indexes = append(indexes, row.Index)
					found = true
					break
				}
			}
		}
		if !found {
			return &ProcessingError{Code: "preparation_issue_invalid"}
		}
	}
	failure, err := r.owner.observer.PreparationFailure(saved.Record(), n, true, lookups, indexes, code, r.lease.manager.now())
	if err != nil {
		return &ProcessingError{Code: "preparation_issue_invalid"}
	}
	if err = r.guard(); err != nil {
		return err
	}
	backend := preparationIssueStorage{discoveryStorage{r.owner.objects, r.reserve}, r.owner.objects}
	store, _ := NewPreparationIssueStore(backend, r.owner.environmentID)
	previous, err := store.Load(r.ctx, saved.Record())
	if err != nil {
		return err
	}
	next, err := previous.Record().Observe(failure)
	if err != nil {
		code := "preparation_issue_invalid"
		if e, ok := err.(*noticeledger.Error); ok && e.Code == "too_large" {
			code = "preparation_issue_capacity"
		}
		return &ProcessingError{Code: code}
	}
	if err = r.guard(); err != nil {
		return err
	}
	actual, _ := previous.Record().Encode()
	expected, _ := next.Encode()
	if bytes.Equal(actual, expected) {
		return r.guard()
	}
	_, err = store.Save(r.ctx, previous, next)
	if err != nil {
		e, ok := err.(*PreparationIssueStorageError)
		if !ok || e == nil || e.Code != "write_uncertain" || !e.WriteUncertain || (e.HTTPStatus >= 400 && e.HTTPStatus < 500) {
			return err
		}
		if err = r.guard(); err != nil {
			return err
		}
		confirmed, err := store.Load(r.ctx, saved.Record())
		if err != nil {
			return &ProcessingError{Code: "issue_confirmation_failed", WriteUncertain: true}
		}
		actual, _ = confirmed.Record().Encode()
		if !bytes.Equal(actual, expected) {
			return &ProcessingError{Code: "issue_confirmation_mismatch", WriteUncertain: true}
		}
	}
	return r.guard()
}

func preparationMembers(resources []routing.Target) []string {
	ids := make([]string, 0, len(resources))
	for _, resource := range resources {
		ids = append(ids, resource.ResourceID)
	}
	return ids
}
