package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

type SavedReviewIssue struct {
	Kind, Code, RevisionDigest, BatchID string
	ResourceIndex                       int
}
type PreparationReview struct {
	Observation PreparationObservation
	Status      string
}
type SavedReview struct {
	Issues      []SavedReviewIssue
	Preparation []PreparationReview
}

// Review derives last-saved evidence, not current dependency health, a complete
// scan or permission to publish/send. Absence must be verified by the caller.
func (r Record) Review(issues PreparationIssues, configurationDigest string) (SavedReview, error) {
	if r.data == nil || !digestPattern.MatchString(configurationDigest) {
		return SavedReview{}, &Error{"invalid_review"}
	}
	if issues.data != nil {
		if err := issues.validate(r); err != nil {
			return SavedReview{}, err
		}
	}
	view := SavedReview{Issues: []SavedReviewIssue{}, Preparation: []PreparationReview{}}
	pending, unassigned := false, false
	for _, v := range r.data.Revisions {
		if recoveryRevisionClosed(r.data, v.Digest) {
			continue
		}
		revisionPending := len(v.Resources) == 0
		for _, row := range v.Resources {
			if row.BatchID != "" || row.Disposition == "CLOSED" || row.Disposition == string(routing.OutOfScope) {
				continue
			}
			revisionPending = true
			if v.Assessment == routing.SupportedShape && r.data.HoldReason == "" && row.Disposition == "UNRESOLVED" {
				view.Issues = append(view.Issues, SavedReviewIssue{Kind: "RESOURCE", Code: row.Reason, RevisionDigest: v.Digest, ResourceIndex: row.Index})
			}
		}
		if revisionPending && v.Assessment == routing.ReviewRequired {
			view.Issues = append(view.Issues, SavedReviewIssue{Kind: "ASSESSMENT", Code: string(v.AssessmentReason), RevisionDigest: v.Digest, ResourceIndex: -1})
		}
		unassigned = unassigned || revisionPending
	}
	pending = unassigned
	for _, b := range r.data.delivery.Batches() {
		if b.State == delivery.APIAccepted || b.State == delivery.Closed {
			continue
		}
		pending = true
		if b.State == delivery.ReviewRequired {
			view.Issues = append(view.Issues, SavedReviewIssue{Kind: "BATCH", Code: string(b.Reason), RevisionDigest: b.Message.Revision.Digest, BatchID: b.Message.BatchID, ResourceIndex: -1})
		}
	}
	if unassigned && r.data.HoldReason != "" {
		view.Issues = append(view.Issues, SavedReviewIssue{Kind: "NOTICE", Code: string(r.data.HoldReason), ResourceIndex: -1})
	}
	if pending && r.data.SourceOutcome != nil && r.data.SourceOutcome.State != SourceAvailable {
		code := "source_unknown"
		if r.data.SourceOutcome.State == SourceUnavailable {
			code = "source_unavailable"
		}
		view.Issues = append(view.Issues, SavedReviewIssue{Kind: "SOURCE", Code: code, ResourceIndex: -1})
	}
	for _, v := range issues.Snapshot().Observations {
		view.Preparation = append(view.Preparation, PreparationReview{v, r.preparationStatus(v, configurationDigest)})
	}
	return view, nil
}

func (r Record) preparationStatus(v PreparationObservation, config string) string {
	revision := r.data.Revisions[revisionIndex(r.data, v.RevisionDigest)]
	row := revision.Resources[v.ResourceIndex]
	if row.BatchID != "" || row.Disposition == "CLOSED" || recoveryRevisionClosed(r.data, v.RevisionDigest) {
		return "HISTORICAL"
	}
	o := r.data.SourceOutcome
	if r.data.HoldReason != "" || row.Disposition != "READY" || revision.RoutingGeneration != v.RoutingGeneration || row.DestinationID != v.DestinationID || preparationContextDigest(config, revision.Source) != v.ContextDigest || o == nil || o.State != SourceAvailable || o.RevisionDigest != v.RevisionDigest {
		return "RECHECK_REQUIRED"
	}
	return "LAST_BLOCKED"
}
