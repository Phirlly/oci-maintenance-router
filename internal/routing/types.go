// Package routing makes provisional ownership decisions without I/O.
// A supported shape or READY group never authorizes delivery. Revision/chain,
// activation, current-source and durable receipt checks belong to orchestration.
package routing

import "oci-maintenance-router/internal/notices"

type AssessmentState string

const (
	Unrelated      AssessmentState = "UNRELATED"
	ReviewRequired AssessmentState = "REVIEW_REQUIRED"
	SupportedShape AssessmentState = "SUPPORTED_SHAPE"
)

type Reason string

const (
	ReasonInvalidPolicy     Reason = "invalid_policy"
	ReasonInvalidNotice     Reason = "invalid_notice"
	ReasonUnrelated         Reason = "unrelated"
	ReasonService           Reason = "unsupported_service"
	ReasonType              Reason = "unsupported_type"
	ReasonPlatform          Reason = "unsupported_platform"
	ReasonNoResources       Reason = "missing_resources"
	ReasonResourceID        Reason = "unsupported_resource_id"
	ReasonUnknownRegion     Reason = "unknown_region"
	ReasonRegionScope       Reason = "region_out_of_scope"
	ReasonLookupMissing     Reason = "lookup_missing"
	ReasonLookupDenied      Reason = "lookup_denied"
	ReasonLookupNotFound    Reason = "lookup_not_found"
	ReasonLookupUnavailable Reason = "lookup_unavailable"
	ReasonIdentityMismatch  Reason = "resource_identity_mismatch"
	ReasonCompartment       Reason = "invalid_compartment"
	ReasonCompartmentScope  Reason = "compartment_out_of_scope"
	ReasonTagMissing        Reason = "tag_missing"
	ReasonTagAmbiguous      Reason = "tag_ambiguous"
	ReasonTagInvalid        Reason = "tag_invalid"
	ReasonMetadataLimit     Reason = "metadata_limit"
	ReasonOwnerUnmapped     Reason = "owner_unmapped"
)

type Assessment struct {
	State  AssessmentState
	Reason Reason
}

type Target struct {
	ResourceID string
	Region     string
}

type Disposition string

const (
	Unresolved    Disposition = "UNRESOLVED"
	Ready         Disposition = "READY"
	OutOfScope    Disposition = "OUT_OF_SCOPE"
	HeldForReview Disposition = "REVIEW_REQUIRED"
)

// Decision identifies malformed resources by normalized index without echoing
// unsafe input. ResourceID is populated only for a supported safe OCID shape.
type Decision struct {
	Index         int
	ResourceID    string
	Region        string
	Disposition   Disposition
	Reason        Reason
	DestinationID string
}

type Group struct {
	DestinationID string
	Resources     []Target
}

// Plan is caller-owned provisional work, not immutable delivery batches or state.
// It deliberately contains no tags, source text, secret references or key values.
type Plan struct {
	Revision          notices.RevisionKey
	RoutingGeneration string
	Assessment        Assessment
	Decisions         []Decision
	Groups            []Group
}

type LookupState string

const (
	Found       LookupState = "FOUND"
	Denied      LookupState = "DENIED"
	NotFound    LookupState = "NOT_FOUND"
	Unavailable LookupState = "UNAVAILABLE"
)

// Metadata is the minimal typed resource ownership projection. The adapter must prove
// fresh, authoritative retrieval and reject malformed wire values. Do not mutate
// input maps/pointers concurrently with Resolve. Nothing here is retained.
type Metadata struct {
	ID            *string
	CompartmentID *string
	DefinedTags   map[string]map[string]any
	FreeformTags  map[string]string
}

type Lookup struct {
	State    LookupState
	Metadata Metadata
}
