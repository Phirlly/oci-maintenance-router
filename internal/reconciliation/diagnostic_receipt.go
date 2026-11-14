package reconciliation

import "oci-maintenance-router/internal/configuration"

// DiagnosticReceipt is a checked operator attestation against saved acceptance.
// It is not independent human authentication or authority to activate or send.
type DiagnosticReceipt struct {
	record     DiagnosticRecord
	configHash [32]byte
}

func (r DiagnosticReceipt) Record() DiagnosticRecord { return r.record }

// DiagnosticReceiptVerifier owns pinned receipt input and performs only reads.
// Bootstrap binds the object backend and real lease to the trusted ledger.
type DiagnosticReceiptVerifier struct {
	input diagnosticControls
	store *DiagnosticStore
}

func NewDiagnosticReceiptVerifier(configBytes, catalogBytes []byte, location configuration.LedgerLocation, releaseSHA256 string, objects DiagnosticObjects) (*DiagnosticReceiptVerifier, error) {
	c, err := readDiagnosticControls(configBytes, catalogBytes, location, releaseSHA256, objects)
	if err != nil {
		return nil, err
	}
	if c.schema != 4 && c.schema != 5 && c.schema != 6 {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	if c.receiptCode == "" {
		return nil, &DiagnosticRunError{Code: "receipt_required"}
	}
	s, _ := NewDiagnosticStore(objects, c.intent.EnvironmentID, c.intent.InstallationID)
	return &DiagnosticReceiptVerifier{c, s}, nil
}
