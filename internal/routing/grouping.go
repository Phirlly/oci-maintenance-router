package routing

import (
	"slices"
	"strings"

	"oci-maintenance-router/internal/notices"
)

// Targets selects bounded direct lookups; it does not perform them.
// knownCandidate is a relevance fact from prior notice/chain observation, not
// update ordering or authorization. The same context must be passed to Resolve.
func (p *Policy) Targets(n notices.Notice, knownCandidate bool) (Assessment, []Target) {
	a, r := p.assess(n, knownCandidate)
	var targets []Target
	if a.State == SupportedShape {
		for i, resource := range r.Resources {
			d := p.scope(i, resource.ResourceID)
			if d.Reason == ReasonLookupMissing {
				targets = append(targets, Target{d.ResourceID, d.Region})
			}
		}
	}
	return a, targets
}

// Resolve recomputes assessment and membership, so supplied metadata cannot add
// recipients or bypass shape/scope checks. Ready groups remain provisional:
// callers still need complete discovery, revision/chain, activation and receipt
// gates before preparing immutable batches or making any delivery attempt.
func (p *Policy) Resolve(n notices.Notice, knownCandidate bool, lookups map[string]Lookup) Plan {
	a, r := p.assess(n, knownCandidate)
	plan := Plan{Revision: n.Key(), Assessment: a}
	if p != nil {
		plan.RoutingGeneration = p.generation
	}
	if a.State == Unrelated {
		return plan
	}
	for i, resource := range r.Resources {
		d := p.scope(i, resource.ResourceID)
		if a.State != SupportedShape {
			d.Disposition, d.Reason = HeldForReview, a.Reason
		} else if d.Reason == ReasonLookupMissing {
			if lookup, exists := lookups[d.ResourceID]; exists {
				d = p.resolveResource(d, lookup)
			}
		}
		plan.Decisions = append(plan.Decisions, d)
	}
	plan.Groups = readyGroups(plan.Decisions)
	return plan
}

func readyGroups(decisions []Decision) []Group {
	byDestination := map[string][]Target{}
	for _, d := range decisions {
		if d.Disposition == Ready {
			byDestination[d.DestinationID] = append(byDestination[d.DestinationID], Target{d.ResourceID, d.Region})
		}
	}
	var groups []Group
	for destination, resources := range byDestination {
		// Decisions follow the normalized resource order. Sort explicitly to keep
		// this output contract independent of the grouping implementation.
		slices.SortFunc(resources, func(a, b Target) int { return strings.Compare(a.ResourceID, b.ResourceID) })
		groups = append(groups, Group{destination, resources})
	}
	slices.SortFunc(groups, func(a, b Group) int { return strings.Compare(a.DestinationID, b.DestinationID) })
	return groups
}
