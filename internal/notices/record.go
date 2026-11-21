// Package notices validates typed source records and identifies content revisions.
// Successful normalization is not permission to route or deliver a notice.
package notices

// Record is a domain input, not an OCI, CLI or Notifications wire schema.
// Adapters must reject invalid wire types and preserve required-field presence.
// Timestamps must be zoned RFC3339 strings; optional text retains nil vs empty.
type Record struct {
	NoticeID              string
	AnnouncementType      string
	ChainID               *string
	Reference             *string
	Services              []string
	Platform              *string
	Summary               *string
	Description           *string
	AdditionalInformation *string
	Times                 [2]SemanticTime
	Resources             []Resource
	TimeCreated           *string
	TimeUpdated           *string
	LifecycleState        *string
}

type SemanticTime struct {
	Type  *string
	Title *string
	Value *string
}

type Resource struct {
	ResourceID           string
	ResourceName         *string
	RegionDisplay        *string
	AdditionalProperties []Property
}

type Property struct {
	Name  *string
	Value *string // An explicit null descriptive value remains nil, distinct from empty.
}

type RevisionKey struct {
	NoticeID string
	Digest   string
}

// Notice owns a validated snapshot. Source text must not be logged or persisted
// wholesale; delivery and state need their own minimal, recipient-scoped data.
type Notice struct {
	record Record
	key    RevisionKey
}

func (n Notice) Key() RevisionKey { return n.key }

// Error contains only controlled field and reason codes, never source values.
type Error struct {
	Field string
	Code  string
}

func (e *Error) Error() string { return e.Field + ": " + e.Code }
