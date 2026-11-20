package reconciliation

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

// SetupConfirmation is immutable saved test evidence, never activation authority.
type SetupConfirmation struct {
	data *setupConfirmationData
	// Only construction from opaque verifier results sets this private bound.
	// Decoding stored bytes cannot manufacture a proposal to create confirmation.
	notBefore time.Time
}

type setupConfirmationData struct {
	SchemaVersion       int    `json:"schemaVersion"`
	EnvironmentID       string `json:"environmentId"`
	InstallationID      string `json:"installationId"`
	DiagnosticRequestID string `json:"diagnosticRequestId"`
	DiagnosticSHA256    string `json:"diagnosticSHA256"`
	OperatorRequestID   string `json:"operatorRequestId"`
	OperatorSHA256      string `json:"operatorSHA256"`
	ConfigurationSHA256 string `json:"configurationSHA256"`
	ConfirmedAt         string `json:"confirmedAt"`
}

func newSetupConfirmation(d DiagnosticReceipt, o OperatorReceipt, at time.Time) (SetupConfirmation, error) {
	ds, os := d.Record().Snapshot(), o.Record().Snapshot()
	if ds.State != DiagnosticAccepted || os.State != OperatorAccepted || d.configHash == ([32]byte{}) || d.configHash != o.configHash || ds.Intent.EnvironmentID != os.Intent.EnvironmentID || ds.Intent.InstallationID != os.Intent.InstallationID {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "invalid_receipts"}
	}
	if !leaseWallTime(at) || at.Before(ds.AcceptedAt) || at.Before(os.AcceptedAt) {
		return SetupConfirmation{}, &SetupConfirmationError{Code: "invalid_time"}
	}
	db, _ := d.Record().Encode()
	ob, _ := o.Record().Encode()
	data := &setupConfirmationData{1, ds.Intent.EnvironmentID, ds.Intent.InstallationID, ds.Intent.RequestID, diagnosticDigest(string(db)), os.Intent.RequestID, diagnosticDigest(string(ob)), hex.EncodeToString(d.configHash[:]), at.UTC().Format(leaseTimeFormat)}
	notBefore := ds.AcceptedAt
	if os.AcceptedAt.After(notBefore) {
		notBefore = os.AcceptedAt
	}
	return SetupConfirmation{data, notBefore}, nil
}

func (r SetupConfirmation) ID() string {
	if r.data == nil {
		return ""
	}
	d := r.data
	preimage, _ := json.Marshal([]string{"setup-confirmation-v1", d.EnvironmentID, d.InstallationID, d.DiagnosticRequestID, d.DiagnosticSHA256, d.OperatorRequestID, d.OperatorSHA256})
	return "sc1-" + diagnosticDigest(string(preimage))
}

func (r SetupConfirmation) confirmedAt() time.Time {
	if r.data == nil {
		return time.Time{}
	}
	at, _ := leaseTimestamp(r.data.ConfirmedAt)
	return at
}
