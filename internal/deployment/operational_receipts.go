package deployment

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/reconciliation"
)

// Any supplied code makes the entire TEST check confirmation-only. Construct all
// required verifiers before returning; incomplete input cannot fall back to sends.
func composeReceiptOperations(s Settings, objects operationalObjects, p pinnedInputs, channel *ocinotifications.Client) (operations, error) {
	location := configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}
	if p.config.OperatorTest != nil {
		paired, err := reconciliation.NewSetupConfirmationCoordinator(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, objects, channel)
		if err != nil {
			return operations{}, err
		}
		return operations{pairedReceipts: paired.Confirm}, nil
	}
	diagnostic, err := reconciliation.NewDiagnosticReceiptVerifier(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, objects)
	if err != nil {
		return operations{}, err
	}
	return operations{diagnosticReceipt: diagnostic.Verify}, nil
}

func (ops operations) confirmReceipts(ctx context.Context, lease *reconciliation.Lease, result *OperationalResult) error {
	if ops.pairedReceipts != nil {
		paired, err := ops.pairedReceipts(ctx, lease)
		if err = pairedReceiptOutcome(ctx, string(paired.Diagnostic.Record().Snapshot().State), string(paired.Operator.Record().Snapshot().State), err, result); err != nil {
			return err
		}
		if paired.Confirmation.ID() == "" {
			return operationFailure("confirmation_not_saved")
		}
		return nil
	}
	diagnostic, err := ops.diagnosticReceipt(ctx, lease)
	result.Diagnostic, err = receiptOutcome(ctx, string(diagnostic.Record().Snapshot().State), err)
	return err
}

// Keep completed verification visible if a later channel or storage step fails.
// This projection cannot establish persistence or grant activation authority.
func pairedReceiptOutcome(ctx context.Context, diagnostic, operator string, err error, result *OperationalResult) error {
	if diagnostic == "" {
		result.Diagnostic, err = receiptOutcome(ctx, diagnostic, err)
		return err
	}
	var failure error
	result.Diagnostic, failure = receiptOutcome(ctx, diagnostic, nil)
	if failure != nil {
		return failure
	}
	if operator == "" {
		result.Operator, err = receiptOutcome(ctx, operator, err)
		return err
	}
	result.Operator, failure = receiptOutcome(ctx, operator, nil)
	if failure != nil {
		return failure
	}
	return err
}

// This is reporting only; it cannot authorize activation or a service call.
func receiptOutcome(ctx context.Context, state string, err error) (*TestOutcome, error) {
	result := &TestOutcome{State: state}
	if err == nil && state != "API_ACCEPTED" {
		err = operationFailure("receipt_not_verified")
	}
	if err != nil {
		result.Code = operationCode(ctx, err)
		return result, err
	}
	result.ReceiptVerified = true
	return result, nil
}
