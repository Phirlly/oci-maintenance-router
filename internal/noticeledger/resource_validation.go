package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func validResource(d Resource, v Revision, hold delivery.Reason, closed bool, version int) bool {
	if (d.ResourceID != "" && !validResourceID(d.ResourceID, version)) || (d.Region != "" && !regionPattern.MatchString(d.Region)) || (d.DestinationID != "" && !destinationPattern.MatchString(d.DestinationID)) || (d.BatchID != "" && !digestPattern.MatchString(d.BatchID)) {
		return false
	}
	complete := d.ResourceID != "" && d.Region != "" && d.DestinationID != ""
	if d.Disposition == "CLOSED" {
		return closed && d.Reason == "operator" && ((d.BatchID != "" && complete) || (d.BatchID == "" && d.DestinationID == "" && (d.ResourceID != "" || d.Region == "")))
	}
	if d.BatchID != "" {
		return complete && (d.Disposition == "ASSIGNED" || d.Disposition == "API_ACCEPTED" || d.Disposition == "REVIEW_REQUIRED")
	}
	if d.ResourceID == "" && (d.Region != "" || d.DestinationID != "") {
		return false
	}
	if hold != "" && d.Disposition != string(routing.OutOfScope) {
		return d.Disposition == "REVIEW_REQUIRED" && d.Reason == string(hold) && d.DestinationID == ""
	}
	if v.Assessment == routing.ReviewRequired {
		return d.Disposition == "REVIEW_REQUIRED" && d.Reason == string(v.AssessmentReason) && d.DestinationID == ""
	}
	switch d.Disposition {
	case "READY":
		return complete && d.Reason == ""
	case "OUT_OF_SCOPE":
		return d.ResourceID != "" && d.Region != "" && d.DestinationID == "" && (d.Reason == string(routing.ReasonRegionScope) || d.Reason == string(routing.ReasonCompartmentScope) || (version == 4 && d.Reason == string(routing.ReasonResourceTypeScope)))
	case "UNRESOLVED":
		if d.DestinationID != "" {
			return false
		}
		if d.ResourceID == "" {
			return d.Reason == string(routing.ReasonResourceID)
		}
		if d.Region == "" {
			return d.Reason == string(routing.ReasonUnknownRegion)
		}
		if version == 4 && d.Reason == string(routing.ReasonService) {
			return true
		}
		switch routing.Reason(d.Reason) {
		case routing.ReasonLookupMissing, routing.ReasonLookupDenied, routing.ReasonLookupNotFound, routing.ReasonLookupUnavailable, routing.ReasonIdentityMismatch, routing.ReasonCompartment, routing.ReasonTagMissing, routing.ReasonTagAmbiguous, routing.ReasonTagInvalid, routing.ReasonMetadataLimit, routing.ReasonOwnerUnmapped:
			return true
		}
	}
	return false
}
