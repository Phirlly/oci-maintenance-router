package reconciliation

import (
	"context"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func (r *RecoveryReviewer) Review(ctx context.Context, id string, kind noticeledger.ActionKind, target string) (result RecoveryReview, err error) {
	if r == nil || r.collector == nil || r.observer == nil || r.policy == nil || r.builder == nil || r.wait == nil || len(id) > 4096 || !recoveryReviewNoticeID.MatchString(id) || !leaseDigest.MatchString(target) || (kind != noticeledger.RetryBatch && kind != noticeledger.ApproveRevision) {
		return RecoveryReview{}, reviewFailure(ctx, "invalid_request")
	}
	bounded, cancel, startErr := discoveryContext(ctx)
	if startErr != nil {
		return RecoveryReview{}, &RecoveryReviewError{Code: string(startErr.Code)}
	}
	ctx = bounded
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			result, err = RecoveryReview{}, reviewFailure(ctx, "canceled")
		}
	}()
	collection, failure := r.collector.collectBounded(ctx, r.wait, nil)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "collection_failed")
	}
	works, failure := discoveryWorks(collection)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "membership_changed")
	}
	var work *discoveryWork
	for _, w := range works {
		if w.id == id {
			work = w
		}
	}
	if work == nil || work.saved.ObjectName == "" {
		return RecoveryReview{}, reviewFailure(ctx, "review_target_missing")
	}
	if failure = r.before(ctx); failure != nil {
		return RecoveryReview{}, failure
	}
	saved, failure := r.collector.saved.LoadRecord(ctx, work.saved.ObjectName)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "record_read_failed")
	}
	environment, notice := saved.Identity()
	if environment != r.collector.environmentID || notice != id || savedMembership(saved) != work.saved.membership {
		return RecoveryReview{}, reviewFailure(ctx, "membership_changed")
	}
	view := RecoveryReview{Saved: saved}
	for _, component := range collection.Candidates.Components() {
		if component.Fingerprint == work.component {
			view.Component = component
		}
	}
	if view.Component.Fingerprint == "" {
		return RecoveryReview{}, reviewFailure(ctx, "component_missing")
	}
	at, failure := time.Parse(time.RFC3339Nano, saved.Snapshot().UpdatedAt)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "record_invalid")
	}
	projected, failure := migrateReviewRecord(saved, at)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "projection_failed")
	}
	digest := target
	found := false
	if kind == noticeledger.RetryBatch {
		for _, b := range projected.Snapshot().Batches {
			if b.Message.BatchID == target {
				digest, found = b.Message.Revision.Digest, true
			}
		}
	} else {
		for _, v := range projected.Snapshot().Revisions {
			if v.Digest == target {
				found = true
			}
		}
	}
	if !found {
		return RecoveryReview{}, reviewFailure(ctx, "review_target_missing")
	}
	view.Projected = projected
	if work.unavailable != 0 {
		view.Blocker = "source_unavailable"
		return view, nil
	}
	current := false
	wanted := notices.RevisionKey{NoticeID: id, Digest: digest}
	for _, m := range collection.Current {
		if m.KnownCandidate && m.Revision == wanted {
			current = true
		}
	}
	if !current {
		view.Blocker = "source_changed"
		return view, nil
	}
	chain := ""
	for _, m := range view.Component.Members {
		if m.Revision == wanted {
			chain = m.ChainID
		}
	}
	projected, failure = projectReviewObservations(projected, work, at)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "projection_failed")
	}
	if failure = r.before(ctx); failure != nil {
		return RecoveryReview{}, failure
	}
	n, failure := r.collector.announcements.Get(ctx, id)
	if failure != nil || ctx.Err() != nil {
		return RecoveryReview{}, reviewFailure(ctx, "source_read_failed")
	}
	actualChain := ""
	if value := n.Snapshot().ChainID; value != nil {
		actualChain = *value
	}
	if n.Key() != wanted || actualChain != chain {
		return RecoveryReview{}, reviewFailure(ctx, "source_changed")
	}
	lookups, failure := r.currentLookups(ctx, n)
	if failure != nil {
		return RecoveryReview{}, failure
	}
	projected, failure = r.observer.Observe(projected, n, true, lookups, at)
	if failure != nil {
		return RecoveryReview{}, reviewFailure(ctx, "projection_failed")
	}
	view.Projected, view.Routing = projected, r.policy.Resolve(n, true, lookups)
	if reviewNeedsRefresh(saved, projected) {
		view.Blocker = "saved_state_refresh_required"
		return view, nil
	}
	view.Blocker = r.reviewEligibility(projected, n, lookups, view.Routing, kind, target)
	if view.Blocker == "" {
		view.Fingerprint, failure = projected.ReviewFingerprint(kind, target, view.Routing.RoutingGeneration, view.Component.Fingerprint)
		if failure != nil {
			return RecoveryReview{}, reviewFailure(ctx, "review_invalid")
		}
	}
	if ctx.Err() != nil {
		return RecoveryReview{}, reviewFailure(ctx, "canceled")
	}
	return view, nil
}

func (r *RecoveryReviewer) before(ctx context.Context) error {
	if err := r.wait(ctx, 100*time.Millisecond); err != nil || ctx.Err() != nil {
		return reviewFailure(ctx, "wait_failed")
	}
	return nil
}
