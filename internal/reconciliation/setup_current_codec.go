package reconciliation

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"oci-maintenance-router/internal/configuration"
)

const setupCurrentLimit = 4096

// Decoded state is structural evidence only; coordinators independently verify
// selected intents, actual accepted records, current controls and original lease.
func decodeSetupCurrent(body []byte, environment, installation string) (setupCurrent, error) {
	if len(body) > setupCurrentLimit {
		return setupCurrent{}, &SetupError{Code: "too_large"}
	}
	d := &setupCurrentData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return setupCurrent{}, &SetupError{Code: "invalid_json"}
	}
	if d.SchemaVersion != 1 || !leaseDigest.MatchString(d.EnvironmentID) || !diagnosticID.MatchString(d.InstallationID) || d.Attempt < 1 || d.Attempt > configuration.MaxSetupAttempt || !leaseDigest.MatchString(d.DiagnosticIntent) || !leaseDigest.MatchString(d.OperatorIntent) {
		return setupCurrent{}, &SetupError{Code: "invalid_record"}
	}
	diag, operator := setupRequestIDs(d.EnvironmentID, d.InstallationID, d.Attempt)
	if d.EnvironmentID != environment || d.InstallationID != installation || d.DiagnosticRequestID != diag || d.OperatorRequestID != operator {
		return setupCurrent{}, &SetupError{Code: "identity_conflict"}
	}
	if c := d.Confirmation; c != nil && (!strings.HasPrefix(c.ID, "sc1-") || !leaseDigest.MatchString(strings.TrimPrefix(c.ID, "sc1-")) || !leaseDigest.MatchString(c.SHA256)) {
		return setupCurrent{}, &SetupError{Code: "invalid_record"}
	}
	r := setupCurrent{data: d}
	canonical, _ := r.encode()
	if !bytes.Equal(canonical, body) {
		return setupCurrent{}, &SetupError{Code: "noncanonical"}
	}
	return r, nil
}
