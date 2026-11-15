package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

// A missing/provisional observation is UNKNOWN. Only positive retained evidence
// can remove a previously reported subject; list absence is never passed here.
func reportIssueResolved(i reportIssue, s noticeledger.Snapshot, view noticeledger.SavedReview, preparationKnown bool) bool {
	switch i.Kind {
	case "STORE":
		return i.Code == "notice_unreadable" || preparationKnown
	case "SOURCE":
		return s.SourceOutcome != nil && s.SourceOutcome.State == noticeledger.SourceAvailable || reportWorkTerminal(s)
	case "NOTICE":
		return s.HoldReason == "" || !reportKindPresent(view, "NOTICE", "")
	case "BATCH":
		for _, b := range s.Batches {
			if b.Message.BatchID == i.BatchID {
				return b.State == delivery.APIAccepted || b.State == delivery.Closed
			}
		}
		return false
	}
	if reportRevisionClosed(s, i.Revision) {
		return true
	}
	for _, revision := range s.Revisions {
		if revision.Digest != i.Revision {
			continue
		}
		if i.Kind == "ASSESSMENT" {
			return !reportKindPresent(view, "ASSESSMENT", i.Revision)
		}
		for _, row := range revision.Resources {
			if row.Index != i.ResourceIndex {
				continue
			}
			if row.BatchID != "" || row.Disposition == "CLOSED" || row.Disposition == "OUT_OF_SCOPE" {
				return true
			}
			if i.Kind == "RESOURCE" {
				return s.HoldReason == "" && (row.Disposition == "READY" || row.Disposition == "UNRESOLVED" && row.Reason != "lookup_missing")
			}
			for _, p := range view.Preparation {
				if p.Observation.RevisionDigest == i.Revision && p.Observation.ResourceIndex == i.ResourceIndex {
					return preparationKnown && p.Status == "HISTORICAL"
				}
			}
		}
	}
	return false
}

func reportKindPresent(view noticeledger.SavedReview, kind, revision string) bool {
	for _, i := range view.Issues {
		if i.Kind == kind && i.RevisionDigest == revision {
			return true
		}
	}
	return false
}

func reportRevisionClosed(s noticeledger.Snapshot, revision string) bool {
	if s.Recovery != nil {
		for _, a := range s.Recovery.Actions {
			if a.Status == noticeledger.RecoveryClosed && a.Request.Kind == noticeledger.CloseRevision && a.Request.TargetID == revision {
				return true
			}
		}
	}
	return false
}

func reportWorkTerminal(s noticeledger.Snapshot) bool {
	for _, revision := range s.Revisions {
		if reportRevisionClosed(s, revision.Digest) {
			continue
		}
		if len(revision.Resources) == 0 {
			return false
		}
		for _, row := range revision.Resources {
			if row.BatchID == "" && row.Disposition != "CLOSED" && row.Disposition != "OUT_OF_SCOPE" {
				return false
			}
		}
	}
	for _, batch := range s.Batches {
		if batch.State != delivery.APIAccepted && batch.State != delivery.Closed {
			return false
		}
	}
	return len(s.Revisions) > 0
}
