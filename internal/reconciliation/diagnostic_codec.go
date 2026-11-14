package reconciliation

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const diagnosticLimit = 16 << 10

func (r DiagnosticRecord) Encode() ([]byte, error) {
	if r.data == nil {
		return nil, &DiagnosticError{Code: "invalid_record"}
	}
	// Closed constructors/transitions and field limits bound every state below
	// diagnosticLimit, including maximum terminal metadata. No key bytes exist here.
	body, _ := json.Marshal(r.data)
	return body, nil
}

// DecodeDiagnostic validates private canonical state and trusted lookup identity,
// not its provenance, persisted status or authority to dispatch/activate.
func DecodeDiagnostic(body []byte, environment, installation, request string) (DiagnosticRecord, error) {
	if len(body) > diagnosticLimit {
		return DiagnosticRecord{}, &DiagnosticError{Code: "too_large"}
	}
	d := &diagnosticData{}
	if !utf8.Valid(body) || json.Unmarshal(body, d) != nil {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_json"}
	}
	if !validDiagnosticData(d) {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_record"}
	}
	if d.Intent.EnvironmentID != environment || d.Intent.InstallationID != installation || d.Intent.RequestID != request {
		return DiagnosticRecord{}, &DiagnosticError{Code: "identity_mismatch"}
	}
	canonical, _ := json.Marshal(d)
	if !bytes.Equal(body, canonical) {
		return DiagnosticRecord{}, &DiagnosticError{Code: "noncanonical"}
	}
	return DiagnosticRecord{data: d}, nil
}
