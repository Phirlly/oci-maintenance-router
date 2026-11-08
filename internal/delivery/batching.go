package delivery

import (
	"slices"
	"strings"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// Application limits, not customer settings or PagerDuty's provider ceiling.
const maxResourcesPerDraft = 50
const maxEnvelopeBytes = 32 * 1024

func (b *Builder) pack(r notices.Record, plan routing.Plan, groups []routing.Group) Result {
	var result Result
	// Check shared content once. Fixed-width dedup identity makes this a lower
	// bound for every nonempty draft; do not repeatedly encode huge shared text.
	common := payload(r, plan.Revision, b.environmentName, "omr1:"+strings.Repeat("0", 64), nil)
	if len(common) > maxEnvelopeBytes {
		for _, group := range groups {
			for _, resource := range group.Resources {
				result.tooLarge(resource)
			}
		}
		return result
	}
	for _, group := range groups {
		var current []routing.Target
		var fitting Draft
		for _, resource := range group.Resources {
			if len(current) == maxResourcesPerDraft {
				result.Drafts = append(result.Drafts, fitting)
				current = nil
			}
			candidate := b.draft(r, plan, group.DestinationID, append(current, resource))
			if len(candidate.snapshot.PayloadWithoutRoutingKey) > maxEnvelopeBytes && len(current) > 0 {
				result.Drafts = append(result.Drafts, fitting)
				current = nil
				candidate = b.draft(r, plan, group.DestinationID, []routing.Target{resource})
			}
			if len(candidate.snapshot.PayloadWithoutRoutingKey) > maxEnvelopeBytes {
				result.tooLarge(resource)
				continue
			}
			current = append(current, resource)
			fitting = candidate
		}
		if len(current) > 0 {
			result.Drafts = append(result.Drafts, fitting)
		}
	}
	return result
}

func (b *Builder) draft(r notices.Record, plan routing.Plan, destination string, resources []routing.Target) Draft {
	id := batchID(b.environmentID, plan.Revision, destination, resources)
	dedup := "omr1:" + id
	data := payload(r, plan.Revision, b.environmentName, dedup, resources)
	return Draft{snapshot: Snapshot{
		BatchID: id, EnvironmentID: b.environmentID, Revision: plan.Revision,
		RoutingGeneration: plan.RoutingGeneration, DestinationID: destination,
		SecretID: b.secrets[destination], PagerDutyRegion: b.pagerDutyRegion,
		Resources: slices.Clone(resources), DedupKey: dedup,
		PayloadWithoutRoutingKey: data, PayloadDigest: digest(data),
	}}
}

func (r *Result) tooLarge(resource routing.Target) {
	r.Issues = append(r.Issues, CapacityIssue{ResourceID: resource.ResourceID, Code: "payload_too_large"})
}
