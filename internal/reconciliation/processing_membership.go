package reconciliation

import (
	"oci-maintenance-router/internal/notices"
	"slices"
)

type processingNoticeWork struct {
	id, membership    string
	component         string
	stable, available bool
	members           []CandidateRevision
	current           map[notices.RevisionKey]bool
	member            CandidateRevision
	eligible          bool
	reason            string
}

func processingWork(initial, final DiscoveryResult) ([]processingNoticeWork, error) {
	old := map[CandidateRevision]bool{}
	stable := map[string]bool{}
	for _, c := range initial.view.components {
		for _, m := range c.Members {
			old[m] = true
		}
		stable[c.Fingerprint] = true
	}
	current := map[notices.RevisionKey]bool{}
	currentIDs := map[string]bool{}
	for _, m := range final.view.current {
		if m.KnownCandidate {
			current[m.Revision] = true
			currentIDs[m.Revision.NoticeID] = true
		}
	}
	available := map[string]bool{}
	for _, u := range final.availability.unavailable {
		available[u.NoticeID] = true
	}
	byID := map[string][]CandidateRevision{}
	works := map[string]processingNoticeWork{}
	for _, c := range final.view.components {
		for _, m := range c.Members {
			delete(old, m)
			byID[m.Revision.NoticeID] = append(byID[m.Revision.NoticeID], m)
			reason := "source_changed"
			eligible := len(c.Members) == 1 && stable[c.Fingerprint] && current[m.Revision] && !available[m.Revision.NoticeID]
			if len(c.Members) == 1 && !stable[c.Fingerprint] {
				reason = "new_candidate_deferred"
			}
			if available[m.Revision.NoticeID] || !currentIDs[m.Revision.NoticeID] {
				reason = "source_unavailable"
			}
			works[m.Revision.NoticeID] = processingNoticeWork{id: m.Revision.NoticeID, member: m, eligible: eligible, reason: reason,
				component: c.Fingerprint, stable: stable[c.Fingerprint], available: currentIDs[m.Revision.NoticeID] && !available[m.Revision.NoticeID], current: current}
		}
	}
	if len(old) != 0 {
		return nil, &ProcessingError{Code: "membership_lost"}
	}
	ids := make([]string, 0, len(works))
	for id := range works {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result := make([]processingNoticeWork, 0, len(ids))
	for _, id := range ids {
		work := works[id]
		slices.SortFunc(byID[id], compareCandidateRevisions)
		work.membership = componentFingerprint(final.environmentID, slices.Compact(byID[id]))
		work.members = slices.Clone(byID[id])
		result = append(result, work)
	}
	return result, nil
}
