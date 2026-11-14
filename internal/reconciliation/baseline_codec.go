package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// DecodeBaseline validates the private canonical representation and expected
// lookup identity. It does not attest prior discovery or trusted object access.
func DecodeBaseline(data []byte, environment, id string) (Baseline, error) {
	if len(data) > baselineLimit {
		return Baseline{}, &BaselineError{Code: "too_large"}
	}
	if !utf8.Valid(data) || !boundedBaselineJSON(data) {
		return Baseline{}, &BaselineError{Code: "invalid_json"}
	}
	d := &baselineData{}
	if json.Unmarshal(data, d) != nil {
		return Baseline{}, &BaselineError{Code: "invalid_schema"}
	}
	if err := validateBaseline(d); err != nil {
		return Baseline{}, err
	}
	if d.EnvironmentID != environment || d.BaselineID != id {
		return Baseline{}, &BaselineError{Code: "identity_mismatch"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(data, canonical) {
		return Baseline{}, &BaselineError{Code: "noncanonical"}
	}
	return Baseline{data: d}, nil
}
