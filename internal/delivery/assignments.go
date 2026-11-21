package delivery

import (
	"slices"

	"oci-maintenance-router/internal/notices"
)

func validateAssignments(batches []savedBatch) error {
	assigned := map[string]map[string]bool{}
	for _, b := range batches {
		members := assigned[b.Message.RevisionDigest]
		if members == nil {
			members = map[string]bool{}
			assigned[b.Message.RevisionDigest] = members
		}
		for _, r := range b.Message.Resources {
			if members[r.ResourceID] {
				return &Error{"assignments", "overlap"}
			}
			members[r.ResourceID] = true
		}
		if len(members) > 256 {
			return &Error{"assignments", "too_large"}
		}
	}
	return nil
}

// Unassigned filters assignments, not eligibility. Pass its result through
// Builder.Build and the future source/activation checks before preparing work.
func (r Record) Unassigned(revision notices.RevisionKey, candidates []string) ([]string, error) {
	if r.data == nil {
		return nil, &Error{"record", "invalid"}
	}
	if revision.NoticeID != r.data.NoticeID || !recordDigest.MatchString(revision.Digest) {
		return nil, &Error{"revision", "invalid"}
	}
	if len(candidates) > 256 {
		return nil, &Error{"selection", "too_large"}
	}
	total := 0
	for _, id := range candidates {
		total += len(id)
		if total > maxRecordBytes {
			return nil, &Error{"selection", "too_large"}
		}
	}
	assigned := map[string]bool{}
	for _, b := range r.data.Batches {
		if b.Message.RevisionDigest == revision.Digest {
			for _, member := range b.Message.Resources {
				assigned[member.ResourceID] = true
			}
		}
	}
	seen := map[string]bool{}
	var result []string
	for _, id := range candidates {
		if seen[id] || !validResourceID(id, r.data.SchemaVersion) {
			return nil, &Error{"selection", "invalid"}
		}
		seen[id] = true
		if !assigned[id] {
			result = append(result, id)
		}
	}
	slices.Sort(result)
	return result, nil
}
