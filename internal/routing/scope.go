package routing

import "regexp"

// Supported safe OCID shapes, not a claim to validate the service's full grammar.
var (
	dbID          = regexp.MustCompile(`^ocid1\.mysqldbsystem\.oc1\.([a-z0-9][a-z0-9-]*)\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
	compartmentID = regexp.MustCompile(`^ocid1\.compartment\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
)

func (p *Policy) scope(index int, id string) Decision {
	d := Decision{Index: index, Disposition: Unresolved, Reason: ReasonResourceID}
	parts := dbID.FindStringSubmatch(id)
	if parts == nil {
		return d // Do not echo malformed or unsupported input.
	}
	d.ResourceID = id
	if p != nil {
		d.Region = p.regions[parts[1]]
	}
	if d.Region == "" {
		d.Reason = ReasonUnknownRegion
	} else if !p.allowed[d.Region] {
		d.Disposition, d.Reason = OutOfScope, ReasonRegionScope
	} else {
		d.Reason = ReasonLookupMissing
	}
	return d
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
