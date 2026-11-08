package routing

import (
	"strings"

	"oci-maintenance-router/internal/notices"
)

func (p *Policy) assess(n notices.Notice, knownCandidate bool) (Assessment, notices.Record) {
	r := n.Snapshot()
	if p == nil || p.generation == "" {
		return Assessment{ReviewRequired, ReasonInvalidPolicy}, r
	}
	if n.Key().NoticeID == "" {
		return Assessment{ReviewRequired, ReasonInvalidNotice}, r
	}
	serviceMatch := false
	for _, service := range r.Services {
		serviceMatch = serviceMatch || p.services[service]
	}
	candidate := knownCandidate || serviceMatch
	for _, resource := range r.Resources {
		candidate = candidate || strings.HasPrefix(resource.ResourceID, "ocid1.mysqldbsystem.")
	}
	if !candidate {
		return Assessment{Unrelated, ReasonUnrelated}, r
	}
	if !serviceMatch {
		return Assessment{ReviewRequired, ReasonService}, r
	}
	if r.AnnouncementType != "SCHEDULED_MAINTENANCE" {
		return Assessment{ReviewRequired, ReasonType}, r
	}
	if r.Platform == nil || *r.Platform != "IAAS" {
		return Assessment{ReviewRequired, ReasonPlatform}, r
	}
	if len(r.Resources) == 0 {
		return Assessment{ReviewRequired, ReasonNoResources}, r
	}
	return Assessment{State: SupportedShape}, r
}
