package reconciliation

import (
	"slices"
	"strings"
)

// Resolve computes relevance independently of admission order, preserving
// current observation order and duplicates. Subsequent additions do not mutate
// returned matches. This bounded pure operation has no I/O or deadline reset;
// a collector must retain and check its original discovery deadline.
func (c *Candidates) Resolve() ([]CandidateMatch, error) {
	v, err := c.ResolveView()
	if err != nil {
		return nil, err
	}
	return v.Current(), nil
}

// ResolveView preserves current occurrences separately from sorted unique
// current/saved membership. Only seeded components are included; saved-only
// components are retained. It has the same bounded, single-owner scope as Resolve.
func (c *Candidates) ResolveView() (CandidateView, error) {
	if err := c.valid(); err != nil {
		return CandidateView{}, err
	}
	components := candidateComponents{parent: make([]int, len(c.facts)), size: make([]int, len(c.facts))}
	for i := range c.facts {
		components.parent[i], components.size[i] = i, 1
	}
	ids, chains := map[string]int{}, map[string]int{}
	for i, fact := range c.facts {
		if prior, ok := ids[fact.revision.NoticeID]; ok {
			components.join(i, prior)
		} else {
			ids[fact.revision.NoticeID] = i
		}
		if fact.chain != "" {
			if prior, ok := chains[fact.chain]; ok {
				components.join(i, prior)
			} else {
				chains[fact.chain] = i
			}
		}
	}
	seeded := make([]bool, len(c.facts))
	for i, fact := range c.facts {
		if fact.seed {
			seeded[components.root(i)] = true
		}
	}
	result := []CandidateMatch{}
	groups := map[int][]CandidateRevision{}
	for i, fact := range c.facts {
		root := components.root(i)
		if fact.current {
			result = append(result, CandidateMatch{Revision: fact.revision, KnownCandidate: seeded[root]})
		}
		if seeded[root] {
			groups[root] = append(groups[root], CandidateRevision{Revision: fact.revision, ChainID: fact.chain})
		}
	}
	resolved := []CandidateComponent{}
	for _, members := range groups {
		slices.SortFunc(members, compareCandidateRevisions)
		members = slices.Compact(members)
		resolved = append(resolved, CandidateComponent{Fingerprint: componentFingerprint(c.environmentID, members), Members: members})
	}
	slices.SortFunc(resolved, func(a, b CandidateComponent) int { return compareCandidateRevisions(a.Members[0], b.Members[0]) })
	return CandidateView{environmentID: c.environmentID, current: result, components: resolved}, nil
}

func compareCandidateRevisions(a, b CandidateRevision) int {
	if order := strings.Compare(a.Revision.NoticeID, b.Revision.NoticeID); order != 0 {
		return order
	}
	if order := strings.Compare(a.Revision.Digest, b.Revision.Digest); order != 0 {
		return order
	}
	return strings.Compare(a.ChainID, b.ChainID)
}

type candidateComponents struct{ parent, size []int }

func (c candidateComponents) root(i int) int {
	for c.parent[i] != i {
		c.parent[i] = c.parent[c.parent[i]]
		i = c.parent[i]
	}
	return i
}

func (c candidateComponents) join(a, b int) {
	a, b = c.root(a), c.root(b)
	if a == b {
		return
	}
	if c.size[a] < c.size[b] {
		a, b = b, a
	}
	c.parent[b] = a
	c.size[a] += c.size[b]
}
