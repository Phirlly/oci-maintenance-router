package reconciliation

import (
	"slices"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

// Compare each record independently: union membership can hide lost saved history.
func savedMembership(r noticeledger.Record) string {
	snapshot := r.Snapshot()
	members := make([]CandidateRevision, 0, len(snapshot.Revisions))
	for _, revision := range snapshot.Revisions {
		member := CandidateRevision{Revision: notices.RevisionKey{NoticeID: snapshot.NoticeID, Digest: revision.Digest}}
		if revision.Source.ChainID != nil {
			member.ChainID = *revision.Source.ChainID
		}
		members = append(members, member)
	}
	slices.SortFunc(members, compareCandidateRevisions)
	return componentFingerprint(snapshot.EnvironmentID, slices.Compact(members))
}

type discoveryWork struct {
	id           string
	saved        SavedNoticeRef
	observations []noticeledger.Observation
	unavailable  int
	hold         bool
	component    string
}

// Only the coordinator's own complete collection reaches this grouping step.
func discoveryWorks(c Collection) ([]*discoveryWork, error) {
	byID := map[string]*discoveryWork{}
	get := func(id string) *discoveryWork {
		if byID[id] == nil {
			byID[id] = &discoveryWork{id: id}
		}
		return byID[id]
	}
	for _, ref := range c.Saved {
		work := get(ref.NoticeID)
		if work.saved.NoticeID != "" && work.saved.membership != ref.membership {
			return nil, &DiscoveryError{Code: "membership_changed"}
		}
		work.saved = ref
	}
	for _, observation := range c.Observations {
		work := get(observation.Key().NoticeID)
		work.observations = append(work.observations, observation)
	}
	for _, unavailable := range c.Availability.unavailable {
		get(unavailable.NoticeID).unavailable = unavailable.HTTPStatus
	}
	for _, component := range c.Candidates.components {
		for _, member := range component.Members {
			work := get(member.Revision.NoticeID)
			work.component = component.Fingerprint
			work.hold = len(component.Members) > 1
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	work := make([]*discoveryWork, 0, len(ids))
	for _, id := range ids {
		work = append(work, byID[id])
	}
	return work, nil
}
