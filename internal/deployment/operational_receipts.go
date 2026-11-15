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
	diagnostic, err := reconciliation.NewDiagnosticReceiptVerifier(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, objects)
	if err != nil {
		return operations{}, err
	}
	result := operations{diagnosticReceipt: diagnostic.Verify}
	if p.config.OperatorTest != nil {
		operator, err := reconciliation.NewOperatorReceiptVerifier(p.configBytes, p.catalogBytes, location, objects, channel)
		if err != nil {
			return operations{}, err
		}
		result.operatorReceipt = operator.Verify
	}
	return result, nil
}

func (ops operations) confirmReceipts(ctx context.Context, lease *reconciliation.Lease, result *OperationalResult) error {
	diagnostic, err := ops.diagnosticReceipt(ctx, lease)
	result.Diagnostic, err = receiptOutcome(ctx, string(diagnostic.Record().Snapshot().State), err)
	if err != nil || ops.operatorReceipt == nil {
		return err
	}
	operator, err := ops.operatorReceipt(ctx, lease)
	result.Operator, err = receiptOutcome(ctx, string(operator.Record().Snapshot().State), err)
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
