package reconciliation

// Resolve computes relevance independently of admission order, preserving
// current observation order and duplicates. Subsequent additions do not mutate
// returned matches. This bounded pure operation has no I/O or deadline reset;
// a collector must retain and check its original discovery deadline.
func (c *Candidates) Resolve() ([]CandidateMatch, error) {
	if err := c.valid(); err != nil {
		return nil, err
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
	for i, fact := range c.facts {
		if fact.current {
			result = append(result, CandidateMatch{Revision: fact.revision, KnownCandidate: seeded[components.root(i)]})
		}
	}
	return result, nil
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
