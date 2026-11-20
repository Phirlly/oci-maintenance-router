package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const setupConfirmationLimit = 4096

func (r SetupConfirmation) Encode() ([]byte, error) {
	if r.data == nil {
		return nil, &SetupConfirmationError{Code: "invalid_record"}
	}
	return json.Marshal(r.data)
}

// DecodeSetupConfirmation checks structural validity and exact lookup identity.
// It does not establish receipt, current configuration or activation authority.
func DecodeSetupConfirmation(body []byte, environment, installation, id string) (SetupConfirmation, error) {
	if len(body) > setupConfirmationLimit {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "too_large"}
	}
	d := &setupConfirmationData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "invalid_json"}
	}
	_, validTime := leaseTimestamp(d.ConfirmedAt)
	if d.SchemaVersion != 1 || !leaseDigest.MatchString(d.EnvironmentID) || !diagnosticID.MatchString(d.InstallationID) || !diagnosticID.MatchString(d.DiagnosticRequestID) || !diagnosticID.MatchString(d.OperatorRequestID) || !leaseDigest.MatchString(d.DiagnosticSHA256) || !leaseDigest.MatchString(d.OperatorSHA256) || !leaseDigest.MatchString(d.ConfigurationSHA256) || !validTime {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "invalid_record"}
	}
	r := SetupConfirmation{data: d}
	if d.EnvironmentID != environment || d.InstallationID != installation || r.ID() != id {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "identity_mismatch"}
	}
	canonical, _ := r.Encode()
	if !bytes.Equal(body, canonical) {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "noncanonical"}
	}
	return r, nil
}
