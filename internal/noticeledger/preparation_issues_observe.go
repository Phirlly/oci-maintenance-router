package noticeledger

import "slices"

// Observe retains first-ever observation time and updates only the subject's
// latest cause/context. It does not resolve or delete historical evidence.
func (p PreparationIssues) Observe(f PreparationFailure) (PreparationIssues, error) {
	if f.notice.data == nil || len(f.observations) == 0 {
		return PreparationIssues{}, &Error{"invalid_preparation_failure"}
	}
	if p.data != nil {
		if err := p.validate(f.notice); err != nil {
			return PreparationIssues{}, err
		}
	}
	next := p.Snapshot()
	if p.data == nil {
		next = PreparationIssueSnapshot{1, f.notice.data.EnvironmentID, preparationNoticeDigest(f.notice.data.NoticeID), []PreparationObservation{}}
	}
	for _, observation := range f.observations {
		i := slices.IndexFunc(next.Observations, func(old PreparationObservation) bool {
			return old.RevisionDigest == observation.RevisionDigest && old.ResourceIndex == observation.ResourceIndex
		})
		if i >= 0 {
			old := next.Observations[i]
			if observation.LastSeenAt < old.LastSeenAt {
				return PreparationIssues{}, &Error{"invalid_time"}
			}
			observation.FirstSeenAt = old.FirstSeenAt
			next.Observations[i] = observation
		} else {
			next.Observations = append(next.Observations, observation)
		}
	}
	slices.SortFunc(next.Observations, comparePreparation)
	result := PreparationIssues{data: &next, parent: p.data, proposal: true}
	if err := result.validate(f.notice); err != nil {
		return PreparationIssues{}, err
	}
	return result, nil
}

func comparePreparation(a, b PreparationObservation) int {
	if a.RevisionDigest < b.RevisionDigest {
		return -1
	}
	if a.RevisionDigest > b.RevisionDigest {
		return 1
	}
	return a.ResourceIndex - b.ResourceIndex
}
