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
		if p.selected == nil {
			candidate = candidate || strings.HasPrefix(resource.ResourceID, "ocid1.mysqldbsystem.")
		} else {
			for kind := range p.selected {
				candidate = candidate || strings.HasPrefix(resource.ResourceID, "ocid1."+kind+".")
			}
		}
	}
	if !candidate {
		return Assessment{Unrelated, ReasonUnrelated}, r
	}
	if p.selected == nil && !serviceMatch {
		return Assessment{ReviewRequired, ReasonService}, r
	}
	if p.selected == nil && r.AnnouncementType != "SCHEDULED_MAINTENANCE" {
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
