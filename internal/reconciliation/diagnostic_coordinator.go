package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/pagerduty"
)

type diagnosticCredential interface {
	Version() int64
	Bytes() []byte
}

// DiagnosticCoordinator owns one pinned TEST intent. Bootstrap binds the Vault
// client, object backend and lease to the trusted region and ledger location.
// It does not process maintenance or establish readiness for LIVE activation.
type DiagnosticCoordinator struct {
	objects  DiagnosticObjects
	intent   DiagnosticIntent
	controls currentControls
	current  func(context.Context, string) (diagnosticCredential, error)
	send     func(context.Context, []byte, []byte) (pagerduty.Response, error)
	token    func() string
}

// DiagnosticResult retains only confirmed saved state, even on an operation
// error. Dispatched means Send was called, not proof of wire delivery. An observed
// 202 is not durable acceptance unless Record has state API_ACCEPTED; neither
// proves human receipt. Existing results have no dispatch or observed status.
type DiagnosticResult struct {
	record     DiagnosticRecord
	dispatched bool
	status     int
}

func (r DiagnosticResult) Record() DiagnosticRecord { return r.record }
func (r DiagnosticResult) Dispatched() bool         { return r.dispatched }
func (r DiagnosticResult) ObservedStatus() int      { return r.status }

func NewDiagnosticCoordinator(configBytes, catalogBytes []byte, location configuration.LedgerLocation, releaseSHA256 string, objects DiagnosticObjects, vault *ocisecrets.Client) (*DiagnosticCoordinator, error) {
	if vault == nil || !leaseDigest.MatchString(releaseSHA256) {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	config, err := configuration.Decode(configBytes, catalog)
	if err != nil || config.SchemaVersion != 3 {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	if config.Mode != "TEST" {
		return nil, &DiagnosticRunError{Code: "intent_required"}
	}
	environment, err := config.EnvironmentID(location)
	if err != nil {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	if _, err := NewDiagnosticStore(objects, environment, config.InstallationID); err != nil {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	catalogHash := sha256.Sum256(catalogBytes)
	intent := DiagnosticIntent{environment, config.InstallationID, config.Diagnostic.RequestID, releaseSHA256, hex.EncodeToString(catalogHash[:]), config.PagerDutyRegion, config.Diagnostic.SecretID, config.Diagnostic.NonProduction}
	sender, _ := pagerduty.New(config.PagerDutyRegion) // Validated by configuration.
	return &DiagnosticCoordinator{objects: objects, intent: intent,
		controls: currentControls{objects, sha256.Sum256(configBytes), catalogHash},
		current:  func(ctx context.Context, id string) (diagnosticCredential, error) { return vault.GetCurrent(ctx, id) },
		send:     sender.Send, token: newLeaseToken}, nil
}
