package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// DecodeBaselineSelection validates canonical state and identity only. The
// referenced baseline and all operational authority require separate checks.
func DecodeBaselineSelection(body []byte, environment, installation string) (BaselineSelection, error) {
	if len(body) > 4096 {
		return BaselineSelection{}, &BaselineError{Code: "too_large"}
	}
	d := &baselineSelectionData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return BaselineSelection{}, &BaselineError{Code: "invalid_json"}
	}
	if d.SchemaVersion != 1 || !leaseDigest.MatchString(d.EnvironmentID) || !baselineID.MatchString(d.InstallationID) || d.Selected == nil || !baselineID.MatchString(d.Selected.ID) || !leaseDigest.MatchString(d.Selected.Digest) {
		return BaselineSelection{}, &BaselineError{Code: "invalid_record"}
	}
	if d.EnvironmentID != environment || d.InstallationID != installation {
		return BaselineSelection{}, &BaselineError{Code: "identity_mismatch"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(body, canonical) {
		return BaselineSelection{}, &BaselineError{Code: "noncanonical"}
	}
	return BaselineSelection{data: d}, nil
}
