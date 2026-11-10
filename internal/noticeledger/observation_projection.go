package noticeledger

import (
	"strings"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func projectRevision(n notices.Notice, plan routing.Plan) Revision {
	v := Revision{Digest: strings.Clone(n.Key().Digest), Source: sourceContext(n.Snapshot()),
		RoutingGeneration: strings.Clone(plan.RoutingGeneration), Assessment: routing.AssessmentState(strings.Clone(string(plan.Assessment.State))),
		AssessmentReason: routing.Reason(strings.Clone(string(plan.Assessment.Reason))), Resources: []Resource{}}
	for _, d := range plan.Decisions {
		v.Resources = append(v.Resources, Resource{Index: d.Index, ResourceID: strings.Clone(d.ResourceID), Region: strings.Clone(d.Region),
			Disposition: strings.Clone(string(d.Disposition)), Reason: strings.Clone(string(d.Reason)), DestinationID: strings.Clone(d.DestinationID)})
	}
	return v
}

func cloneText(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.Clone(*s)
	return &v
}

func sourceContext(r notices.Record) Source {
	// Clone strings as well as pointers: a short value may share the raw input's
	// backing allocation. Do not retain discarded source text through that value.
	s := Source{ChainID: cloneText(r.ChainID), Reference: cloneText(r.Reference), TimeCreated: cloneText(r.TimeCreated), TimeUpdated: cloneText(r.TimeUpdated)}
	for i, t := range r.Times {
		s.Times[i] = SemanticTime{cloneText(t.Type), cloneText(t.Title), cloneText(t.Value)}
	}
	return s
}
