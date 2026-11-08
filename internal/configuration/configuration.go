// Package configuration validates deployment-generated routing settings.
// Successful decoding is not permission to send: readiness and baseline
// completeness must be checked by the activation lifecycle.
package configuration

type Catalog struct {
	Regions       map[string]string
	MySQLServices []string
}

type Config struct {
	SchemaVersion    int
	EnvironmentName  string
	TenancyID        string
	DeploymentRegion string
	RegionMap        map[string]string
	CompartmentIDs   []string
	MySQLServices    []string
	Ownership        Ownership
	Destinations     []Destination
	Routes           []Route
	PagerDutyRegion  string
	IntakeTopicID    string
	OperatorTopicID  string
	Mode             string
	ActiveBaselineID string
	Replay           []Replay
}

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
