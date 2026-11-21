// Package configuration validates deployment-generated routing settings.
// Successful decoding is not permission to send: readiness and baseline
// completeness must be checked by the activation lifecycle.
package configuration

type Catalog struct {
	// Zero denotes the original unversioned MySQL catalog; one is the first
	// explicitly versioned, closed MySQL/Compute catalog.
	SchemaVersion    int
	Regions          map[string]string
	MySQLServices    []string
	ResourceServices map[string][]string
}

type Config struct {
	SchemaVersion    int
	EnvironmentName  string
	TenancyID        string
	DeploymentRegion string
	RegionMap        map[string]string
	CompartmentIDs   []string
	MySQLServices    []string
	ResourceTypes    []string
	// ResourceServices is derived from the independent catalog, never read
	// from customer JSON. It owns only the selected types' complete label lists.
	ResourceServices map[string][]string
	Ownership        Ownership
	Destinations     []Destination
	Routes           []Route
	PagerDutyRegion  string
	IntakeTopicID    string
	OperatorTopicID  string
	Mode             string
	ActiveBaselineID string
	Replay           []Replay
	InstallationID   string
	Preparation      Preparation
	Diagnostic       *DiagnosticRequest
	OperatorTest     *OperatorTestRequest
	Recovery         *RecoveryRequest
	Setup            *SetupRequest
	OperatorChannel  *OperatorChannel
}

// SetupRequest is explicit setup intent, not evidence that a test was delivered.
// NONE carries no transient test controls; saved state owns the selected attempt.
type SetupRequest struct {
	Action                      string
	Attempt                     int64
	SecretID                    string
	NonProduction               bool
	PagerDutyCode, OperatorCode string
}

// OperatorChannel remains configured independently of a transient setup action.
type OperatorChannel struct{ SubscriptionID, Endpoint string }

// OperatorTestRequest is desired EMAIL test intent and optional receipt input.
// It does not prove channel association, publication permission or human receipt.
type OperatorTestRequest struct {
	RequestID, SubscriptionID, Endpoint, ReceiptCode string
}

// DiagnosticRequest is desired test intent and optional receipt attestation,
// never permission to dispatch or activate. The receipt code is not a secret.
type DiagnosticRequest struct {
	RequestID, SecretID string
	NonProduction       bool
	ReceiptCode         string
}

// Preparation contains desired controls only; the coordinator enforces mode,
// pinned configuration, history, CURRENT checks and lease before any operation.
type Preparation struct{ Initialize bool }

type Ownership struct {
	Kind      string
	Namespace string
	Key       string
}

type Destination struct {
	ID       string
	SecretID string
}

type Route struct {
	OwnerValue    string
	DestinationID string
}

type Replay struct {
	NoticeID       string
	RevisionDigest string
}

// LedgerLocation comes from verified bootstrap metadata, not routing JSON.
type LedgerLocation struct {
	Namespace string
	Bucket    string
}

// Error deliberately excludes input values and unknown property names.
type Error struct {
	Field string
	Code  string
}

func (e *Error) Error() string { return e.Field + ": " + e.Code }
