package noticeledger

import (
	"slices"
	"time"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// PreparationFailure recomputes provisional current routing and correlates it
// with retained rows. Held rows remain held; their cleared destinations are not
// treated as current ownership. This operation supplies no approval authority.
func (o *Observer) PreparationFailure(r Record, n notices.Notice, known bool, lookups map[string]routing.Lookup, indexes []int, code string, at time.Time) (PreparationFailure, error) {
	invalid := &Error{"invalid_preparation_failure"}
	if o == nil || o.policy == nil || r.data == nil || r.data.EnvironmentID != o.environmentID || n.Key().NoticeID != r.data.NoticeID || !preparationCode(code) || len(indexes) == 0 || len(indexes) > 256 {
		return PreparationFailure{}, invalid
	}
	i := revisionIndex(r.data, n.Key().Digest)
	if i < 0 || recoveryRevisionClosed(r.data, n.Key().Digest) {
		return PreparationFailure{}, invalid
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return PreparationFailure{}, err
	}
	plan := o.policy.Resolve(n, known, lookups)
	if plan.Assessment.State != routing.SupportedShape {
		return PreparationFailure{}, invalid
	}
	current := projectRevision(n, plan)
	rows := r.data.Revisions[i].Resources
	if len(current.Resources) != len(rows) {
		return PreparationFailure{}, invalid
	}
	ordered := slices.Clone(indexes)
	slices.Sort(ordered)
	items := make([]PreparationObservation, 0, len(ordered))
	contextDigest := preparationContextDigest(o.configurationDigest, current.Source)
	for j, index := range ordered {
		if index < 0 || index >= len(rows) || (j > 0 && index == ordered[j-1]) {
			return PreparationFailure{}, invalid
		}
		old, ready := rows[index], current.Resources[index]
		if old.Index != index || ready.Index != index || old.ResourceID != ready.ResourceID || old.Region != ready.Region || old.BatchID != "" || old.Disposition == "CLOSED" || ready.Disposition != string(routing.Ready) || !destinationPattern.MatchString(ready.DestinationID) {
			return PreparationFailure{}, invalid
		}
		items = append(items, PreparationObservation{n.Key().Digest, index, code, stamp, stamp, contextDigest, current.RoutingGeneration, ready.DestinationID})
	}
	return PreparationFailure{r, items}, nil
}

func preparationCode(code string) bool {
	switch code {
	case "payload_too_large", "secret_unavailable", "secret_invalid", "state_capacity":
		return true
	}
	return false
}
