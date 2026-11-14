package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"oci-maintenance-router/internal/configuration"
)

// Shared derivation keeps dispatch and receipt verification on the same pinned
// identity. Receipt input and routing policy are deliberately outside that intent.
type diagnosticControls struct {
	intent            DiagnosticIntent
	current           currentControls
	schema            int
	mode, receiptCode string
}

func readDiagnosticControls(configBytes, catalogBytes []byte, location configuration.LedgerLocation, release string, objects DiagnosticObjects) (diagnosticControls, error) {
	invalid := &DiagnosticRunError{Code: "invalid_request"}
	if !leaseDigest.MatchString(release) {
		return diagnosticControls{}, invalid
	}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return diagnosticControls{}, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || (c.SchemaVersion != 3 && c.SchemaVersion != 4 && c.SchemaVersion != 5) {
		return diagnosticControls{}, invalid
	}
	if c.Diagnostic == nil {
		return diagnosticControls{}, &DiagnosticRunError{Code: "intent_required"}
	}
	environment, err := c.EnvironmentID(location)
	if err != nil {
		return diagnosticControls{}, invalid
	}
	if _, err := NewDiagnosticStore(objects, environment, c.InstallationID); err != nil {
		return diagnosticControls{}, invalid
	}
	catalogHash := sha256.Sum256(catalogBytes)
	intent := DiagnosticIntent{environment, c.InstallationID, c.Diagnostic.RequestID, release, hex.EncodeToString(catalogHash[:]), c.PagerDutyRegion, c.Diagnostic.SecretID, c.Diagnostic.NonProduction}
	return diagnosticControls{intent, currentControls{objects, sha256.Sum256(configBytes), catalogHash}, c.SchemaVersion, c.Mode, c.Diagnostic.ReceiptCode}, nil
}
