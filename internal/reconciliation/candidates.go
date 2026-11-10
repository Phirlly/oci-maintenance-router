package reconciliation

import (
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// Candidates retains bounded relevance facts for one discovery invocation.
// Use a single owner; concurrent mutation is unsupported. It retains no full
// notices or saved records. Runtime composition owns trusted source binding.
type Candidates struct {
	policy        *routing.Policy
	environmentID string
	facts         []candidateFact
	bytes         int
	failed        Code
}

// CandidateMatch preserves an observed revision occurrence. KnownCandidate is
// relevance only, never proof of complete discovery, freshness or send authority.
type CandidateMatch struct {
	Revision       notices.RevisionKey
	KnownCandidate bool
}

type candidateFact struct {
	revision notices.RevisionKey
	chain    string
	seed     bool
	current  bool
}

func NewCandidates(data []byte, catalog configuration.Catalog, location configuration.LedgerLocation) (*Candidates, error) {
	c, err := configuration.Decode(data, catalog)
	if err != nil {
		return nil, err
	}
	environment, err := c.EnvironmentID(location)
	if err != nil {
		return nil, err
	}
	policy, err := routing.New(data, catalog)
	if err != nil {
		return nil, err
	}
	return &Candidates{policy: policy, environmentID: environment}, nil
}

// Observe extracts one compact fact from a validated current notice. An invalid
// admission permanently fails this accumulator, including any later Resolve.
func (c *Candidates) Observe(n notices.Notice) error {
	if err := c.valid(); err != nil {
		return err
	}
	if n.Key().NoticeID == "" {
		return c.fail(InvalidNotice)
	}
	assessment, _ := c.policy.Targets(n, false)
	f := candidateFact{revision: n.Key(), seed: assessment.State != routing.Unrelated, current: true}
	if link := n.Snapshot().ChainID; link != nil {
		f.chain = *link
	}
	return c.admit([]candidateFact{f})
}

// Restore admits every revision of an environment-bound saved candidate, all
// or nothing. Saved relevance survives routing changes and source retention;
// it does not authorize resuming, clearing a hold or delivering saved work.
func (c *Candidates) Restore(r noticeledger.Record) error {
	if err := c.valid(); err != nil {
		return err
	}
	environment, id := r.Identity()
	if environment == "" || id == "" {
		return c.fail(InvalidRecord)
	}
	if environment != c.environmentID {
		return c.fail(EnvironmentMismatch)
	}
	snapshot := r.Snapshot()
	if len(snapshot.Revisions) > maxCandidateFacts-len(c.facts) {
		return c.fail(ItemLimit)
	}
	facts := make([]candidateFact, len(snapshot.Revisions))
	for i, revision := range snapshot.Revisions {
		facts[i] = candidateFact{revision: notices.RevisionKey{NoticeID: id, Digest: revision.Digest}, seed: true}
		if revision.Source.ChainID != nil {
			facts[i].chain = *revision.Source.ChainID
		}
	}
	return c.admit(facts)
}
