package routing

import "regexp"

// Supported safe OCID shapes, not a claim to validate the service's full grammar.
var compartmentID = regexp.MustCompile(`^ocid1\.compartment\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func (p *Policy) scope(index int, id string, services []string) Decision {
	d := Decision{Index: index, Disposition: Unresolved, Reason: ReasonResourceID}
	kind, region := resourceIdentity(id)
	if kind == "" || ((p == nil || p.selected == nil) && kind != "mysqldbsystem") {
		return d // Do not echo malformed or unsupported input.
	}
	d.ResourceID = id
	if p != nil {
		d.Region = p.regions[region]
	}
	if d.Region == "" {
		d.Reason = ReasonUnknownRegion
	} else if !p.allowed[d.Region] {
		d.Disposition, d.Reason = OutOfScope, ReasonRegionScope
	} else if p.selected != nil && p.selected[kind] == nil {
		d.Disposition, d.Reason = OutOfScope, ReasonResourceTypeScope
	} else if p.selected != nil && !matchesService(p.selected[kind], services) {
		d.Reason = ReasonService
	} else {
		d.Reason = ReasonLookupMissing
	}
	return d
}

func matchesService(approved map[string]bool, labels []string) bool {
	for _, label := range labels {
		if approved[label] {
			return true
		}
	}
	return false
}

func (p *Policy) resolveResource(d Decision, lookup Lookup) Decision {
	switch lookup.State {
	case Denied:
		d.Reason = ReasonLookupDenied
	case NotFound:
		d.Reason = ReasonLookupNotFound
	case Found:
		m := lookup.Metadata
		if m.ID == nil || *m.ID != d.ResourceID {
			d.Reason = ReasonIdentityMismatch
		} else if m.CompartmentID == nil || len(*m.CompartmentID) > maxMetadataText || (*m.CompartmentID != p.tenancyID && !compartmentID.MatchString(*m.CompartmentID)) {
			d.Reason = ReasonCompartment
		} else if !p.compartments[*m.CompartmentID] {
			d.Disposition, d.Reason = OutOfScope, ReasonCompartmentScope
		} else {
			d.DestinationID, d.Reason = p.destination(m)
			if d.Reason == "" {
				d.Disposition = Ready
			}
		}
	default:
		d.Reason = ReasonLookupUnavailable
	}
	return d
}
