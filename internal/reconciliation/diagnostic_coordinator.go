package reconciliation

import (
	"context"
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
	if vault == nil {
		return nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	c, err := readDiagnosticControls(configBytes, catalogBytes, location, releaseSHA256, objects)
	if err != nil {
		return nil, err
	}
	if c.mode != "TEST" {
		return nil, &DiagnosticRunError{Code: "intent_required"}
	}
	sender, _ := pagerduty.New(c.intent.PagerDutyRegion) // Validated by configuration.
	return &DiagnosticCoordinator{objects: objects, intent: c.intent,
		controls: c.current,
		current:  func(ctx context.Context, id string) (diagnosticCredential, error) { return vault.GetCurrent(ctx, id) },
		send:     sender.Send, token: newLeaseToken}, nil
}
