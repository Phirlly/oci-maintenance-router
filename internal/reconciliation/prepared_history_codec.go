package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// DecodePreparedHistory validates canonical state and expected identity, not
// the origin, existence or completeness of the referenced stored baseline.
func DecodePreparedHistory(body []byte, environment, installation string) (PreparedHistory, error) {
	if len(body) > 4096 {
		return PreparedHistory{}, &BaselineError{Code: "too_large"}
	}
	d := &preparedHistoryData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return PreparedHistory{}, &BaselineError{Code: "invalid_json"}
	}
	if d.SchemaVersion != 1 || !leaseDigest.MatchString(d.EnvironmentID) || !baselineID.MatchString(d.InstallationID) ||
		(d.Prepared != nil && (!baselineID.MatchString(d.Prepared.ID) || !leaseDigest.MatchString(d.Prepared.Digest))) {
		return PreparedHistory{}, &BaselineError{Code: "invalid_record"}
	}
	if d.EnvironmentID != environment || d.InstallationID != installation {
		return PreparedHistory{}, &BaselineError{Code: "identity_mismatch"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(body, canonical) {
		return PreparedHistory{}, &BaselineError{Code: "noncanonical"}
	}
	return PreparedHistory{data: d}, nil
}
