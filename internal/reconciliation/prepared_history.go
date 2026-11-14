package reconciliation

import "encoding/json"

// PreparedHistory retains cumulative preparation, never LIVE activation or
// permission to send. Proposals and decoded records provide no I/O authority.
type PreparedHistory struct {
	data   *preparedHistoryData
	parent *preparedHistoryData
}

type preparedBaselineReference struct {
	ID     string `json:"baselineId"`
	Digest string `json:"contentSha256"`
}
type preparedHistoryData struct {
	SchemaVersion  int                        `json:"schemaVersion"`
	EnvironmentID  string                     `json:"environmentId"`
	InstallationID string                     `json:"installationId"`
	Prepared       *preparedBaselineReference `json:"preparedBaseline"`
}

func NewPreparedHistory(environment, installation string) (PreparedHistory, error) {
	if !leaseDigest.MatchString(environment) || !baselineID.MatchString(installation) {
		return PreparedHistory{}, &BaselineError{Code: "invalid_identity"}
	}
	return PreparedHistory{data: &preparedHistoryData{SchemaVersion: 1, EnvironmentID: environment, InstallationID: installation}}, nil
}
func (h PreparedHistory) EnvironmentID() string {
	if h.data == nil {
		return ""
	}
	return h.data.EnvironmentID
}
func (h PreparedHistory) InstallationID() string {
	if h.data == nil {
		return ""
	}
	return h.data.InstallationID
}
func (h PreparedHistory) BaselineID() string {
	if h.data == nil || h.data.Prepared == nil {
		return ""
	}
	return h.data.Prepared.ID
}
func (h PreparedHistory) BaselineDigest() string {
	if h.data == nil || h.data.Prepared == nil {
		return ""
	}
	return h.data.Prepared.Digest
}
func (h PreparedHistory) Encode() ([]byte, error) {
	if h.data == nil {
		return nil, &BaselineError{Code: "invalid_record"}
	}
	return json.Marshal(h.data)
}
