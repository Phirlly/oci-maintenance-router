package noticeledger

import (
	"slices"
	"strings"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
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
	if r.data != nil && (r.data.EnvironmentID != o.environmentID || r.data.NoticeID != n.Key().NoticeID) {
		return Record{}, &Error{"identity_mismatch"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	plan := o.policy.Resolve(n, knownCandidate, lookups)
	if plan.Assessment.State == routing.Unrelated {
		return Record{}, &Error{"unrelated"}
	}
	d := recordData{SchemaVersion: 1, EnvironmentID: o.environmentID, NoticeID: n.Key().NoticeID, Revisions: []Revision{}}
	if r.data != nil {
		d = r.data.copy()
	}
	v := Revision{Digest: n.Key().Digest, Source: sourceContext(n.Snapshot()), FirstSeenAt: stamp, LastSeenAt: stamp, RoutingGeneration: plan.RoutingGeneration, Assessment: plan.Assessment.State, AssessmentReason: plan.Assessment.Reason, Resources: []Resource{}}
	for _, decision := range plan.Decisions {
		v.Resources = append(v.Resources, Resource{Index: decision.Index, ResourceID: decision.ResourceID, Region: decision.Region, Disposition: string(decision.Disposition), Reason: string(decision.Reason), DestinationID: decision.DestinationID})
	}
	i := revisionIndex(&d, v.Digest)
	if i >= 0 {
		previous := d.Revisions[i]
		v.FirstSeenAt = previous.FirstSeenAt
		if len(previous.Resources) != len(v.Resources) {
			return Record{}, &Error{"source_mismatch"}
		}
		for j, old := range previous.Resources {
			if old.BatchID != "" {
				if old.ResourceID != v.Resources[j].ResourceID {
					return Record{}, &Error{"source_mismatch"}
				}
				v.Resources[j] = old
			}
		}
		d.Revisions[i] = v
	} else {
		d.Revisions = append(d.Revisions, v)
	}
	slices.SortFunc(d.Revisions, func(a, b Revision) int { return strings.Compare(a.Digest, b.Digest) })
	d.UpdatedAt = stamp
	if len(d.Revisions) > 1 {
		d.HoldReason = delivery.ReasonSourceChanged
	}
	return r.finalize(d, at)
}

func sourceContext(r notices.Record) Source {
	s := Source{ChainID: r.ChainID, Reference: r.Reference, TimeCreated: r.TimeCreated, TimeUpdated: r.TimeUpdated}
	for i, t := range r.Times {
		s.Times[i] = SemanticTime{t.Type, t.Title, t.Value}
	}
	return s
}
func revisionIndex(d *recordData, digest string) int {
	for i, v := range d.Revisions {
		if v.Digest == digest {
			return i
		}
	}
	return -1
}
