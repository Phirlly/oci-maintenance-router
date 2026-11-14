package reconciliation

import "encoding/json"

// BaselineSelection records a nonempty cumulative reference, not LIVE intent,
// readiness, current scope, persisted target or permission to send.
type BaselineSelection struct {
	data   *baselineSelectionData
	parent *baselineSelectionData
}
type baselineSelectionData struct {
	SchemaVersion  int                `json:"schemaVersion"`
	EnvironmentID  string             `json:"environmentId"`
	InstallationID string             `json:"installationId"`
	Selected       *baselineReference `json:"selectedBaseline"`
}

func NewBaselineSelection(environment, installation string, b Baseline) (BaselineSelection, error) {
	if !leaseDigest.MatchString(environment) || !baselineID.MatchString(installation) {
		return BaselineSelection{}, &BaselineError{Code: "invalid_identity"}
	}
	if b.data == nil || b.EnvironmentID() != environment {
		return BaselineSelection{}, &BaselineError{Code: "invalid_record"}
	}
	ref := referenceForBaseline(b)
	return BaselineSelection{data: &baselineSelectionData{1, environment, installation, &ref}}, nil
}
func (s BaselineSelection) EnvironmentID() string {
	if s.data == nil {
		return ""
	}
	return s.data.EnvironmentID
}
func (s BaselineSelection) InstallationID() string {
	if s.data == nil {
		return ""
	}
	return s.data.InstallationID
}
func (s BaselineSelection) BaselineID() string {
	if s.data == nil {
		return ""
	}
	return s.data.Selected.ID
}
func (s BaselineSelection) BaselineDigest() string {
	if s.data == nil {
		return ""
	}
	return s.data.Selected.Digest
}
func (s BaselineSelection) Encode() ([]byte, error) {
	if s.data == nil {
		return nil, &BaselineError{Code: "invalid_record"}
	}
	return json.Marshal(s.data)
}

// Matches supports a no-write repeat after the coordinator's fresh guards. It
// compares the entire canonical baseline; it does not verify stored state.
func (s BaselineSelection) Matches(b Baseline) bool {
	return s.data != nil && b.data != nil && b.EnvironmentID() == s.EnvironmentID() && referenceForBaseline(b) == *s.data.Selected
}
