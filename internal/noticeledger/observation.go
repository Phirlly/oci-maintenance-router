package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

type Observer struct {
	policy        *routing.Policy
	environmentID string
}

func New(data []byte, catalog configuration.Catalog, location configuration.LedgerLocation) (*Observer, error) {
	c, err := configuration.Decode(data, catalog)
	if err != nil {
		return nil, err
	}
	env, err := c.EnvironmentID(location)
	if err != nil {
		return nil, err
	}
	p, err := routing.New(data, catalog)
	if err != nil {
		return nil, err
	}
	return &Observer{p, env}, nil
}

// Observe recomputes provisional routing. knownCandidate is a discovery fact,
// not proof of complete discovery, activation, source freshness or eligibility.
func (o *Observer) Observe(r Record, n notices.Notice, knownCandidate bool, lookups map[string]routing.Lookup, at time.Time) (Record, error) {
	if o == nil || o.policy == nil || n.Key().NoticeID == "" {
		return Record{}, &Error{"invalid_observation"}
	}
	stamp, err := r.observationTime(o.environmentID, n.Key().NoticeID, at)
	if err != nil {
		return Record{}, err
	}
	plan := o.policy.Resolve(n, knownCandidate, lookups)
	if plan.Assessment.State == routing.Unrelated {
		return Record{}, &Error{"unrelated"}
	}
	return r.mergeObservation(o.environmentID, n.Key().NoticeID, projectRevision(n, plan), stamp, at)
}
