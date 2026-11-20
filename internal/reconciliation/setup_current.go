package reconciliation

import (
	"encoding/json"
	"oci-maintenance-router/internal/configuration"
)

// setupCurrent is the selected explicit attempt, not readiness or send authority.
type setupCurrent struct {
	data, parent *setupCurrentData
}

type setupCurrentData struct {
	SchemaVersion       int                   `json:"schemaVersion"`
	EnvironmentID       string                `json:"environmentId"`
	InstallationID      string                `json:"installationId"`
	Attempt             int64                 `json:"attempt"`
	DiagnosticRequestID string                `json:"diagnosticRequestId"`
	DiagnosticIntent    string                `json:"diagnosticIntentSHA256"`
	OperatorRequestID   string                `json:"operatorRequestId"`
	OperatorIntent      string                `json:"operatorIntentSHA256"`
	Confirmation        *setupConfirmationRef `json:"confirmation,omitempty"`
}

type setupConfirmationRef struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

func setupRequestIDs(environment, installation string, attempt int64) (string, string) {
	d, _ := json.Marshal([]any{"setup-diagnostic-v1", environment, installation, attempt})
	o, _ := json.Marshal([]any{"setup-operator-v1", environment, installation, attempt})
	return "sd1-" + diagnosticDigest(string(d)), "so1-" + operatorDigest(string(o))
}

func newSetupCurrent(attempt int64, diagnostic DiagnosticIntent, operator OperatorIntent) (setupCurrent, error) {
	if attempt < 1 || attempt > configuration.MaxSetupAttempt || !validDiagnosticIntent(diagnostic) || !validOperatorIntent(operator) || diagnostic.EnvironmentID != operator.EnvironmentID || diagnostic.InstallationID != operator.InstallationID {
		return setupCurrent{}, &SetupError{Code: "invalid_request"}
	}
	d, o := setupRequestIDs(diagnostic.EnvironmentID, diagnostic.InstallationID, attempt)
	if diagnostic.RequestID != d || operator.RequestID != o {
		return setupCurrent{}, &SetupError{Code: "identity_conflict"}
	}
	digest, _ := diagnosticIdentity(diagnostic)
	return setupCurrent{data: &setupCurrentData{SchemaVersion: 1, EnvironmentID: diagnostic.EnvironmentID, InstallationID: diagnostic.InstallationID, Attempt: attempt,
		DiagnosticRequestID: d, DiagnosticIntent: digest, OperatorRequestID: o, OperatorIntent: operatorIntentDigest(operator)}}, nil
}

func (r setupCurrent) selectAttempt(attempt int64, diagnostic DiagnosticIntent, operator OperatorIntent) (setupCurrent, error) {
	next, err := newSetupCurrent(attempt, diagnostic, operator)
	if err != nil {
		return setupCurrent{}, err
	}
	if r.data == nil || r.data.EnvironmentID != next.data.EnvironmentID || r.data.InstallationID != next.data.InstallationID {
		return setupCurrent{}, &SetupError{Code: "identity_conflict"}
	}
	if attempt < r.data.Attempt {
		return setupCurrent{}, &SetupError{Code: "attempt_stale"}
	}
	if attempt == r.data.Attempt {
		if next.data.DiagnosticIntent != r.data.DiagnosticIntent || next.data.OperatorIntent != r.data.OperatorIntent {
			return setupCurrent{}, &SetupError{Code: "identity_conflict"}
		}
		return r, nil
	}
	next.parent = r.data
	return next, nil
}

func (r setupCurrent) attach(confirmation SetupConfirmation) (setupCurrent, error) {
	if r.data == nil || confirmation.data == nil {
		return setupCurrent{}, &SetupError{Code: "invalid_request"}
	}
	d := r.data
	body, _ := confirmation.Encode()
	c, err := DecodeSetupConfirmation(body, d.EnvironmentID, d.InstallationID, confirmation.ID())
	if err != nil || c.data.DiagnosticRequestID != d.DiagnosticRequestID || c.data.OperatorRequestID != d.OperatorRequestID {
		return setupCurrent{}, &SetupError{Code: "identity_conflict"}
	}
	ref := setupConfirmationRef{c.ID(), diagnosticDigest(string(body))}
	if d.Confirmation != nil {
		if *d.Confirmation != ref {
			return setupCurrent{}, &SetupError{Code: "identity_conflict"}
		}
		return r, nil
	}
	next := *d
	next.Confirmation = &ref
	return setupCurrent{&next, d}, nil
}

func (r setupCurrent) encode() ([]byte, error) {
	if r.data == nil {
		return nil, &SetupError{Code: "invalid_record"}
	}
	return json.Marshal(r.data)
}
