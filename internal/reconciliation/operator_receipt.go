package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

// OperatorReceipt is checked attestation against saved publication acceptance and
// observed current channel identity. It is neither independent human receipt
// authentication nor authority to activate or send.
type OperatorReceipt struct {
	record     OperatorChallenge
	configHash [32]byte
}

func (r OperatorReceipt) Record() OperatorChallenge { return r.record }

// OperatorReceiptVerifier only reads. Bootstrap binds its concrete client,
// backend and actual lease to the trusted region, principal and ledger location.
type OperatorReceiptVerifier struct {
	input operatorControls
	store *OperatorChallengeStore
	read  func(context.Context) (OperatorChannel, error)
}

func NewOperatorReceiptVerifier(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects OperatorChallengeObjects, client *ocinotifications.Client) (*OperatorReceiptVerifier, error) {
	if client == nil {
		return nil, &OperatorRunError{Code: "invalid_request"}
	}
	c, err := readOperatorControls(configBytes, catalogBytes, location, objects)
	if err != nil {
		return nil, err
	}
	return newOperatorReceiptVerifier(c, objects, client)
}

func newOperatorReceiptVerifier(c operatorControls, objects OperatorChallengeObjects, client *ocinotifications.Client) (*OperatorReceiptVerifier, error) {
	if client == nil {
		return nil, &OperatorRunError{Code: "invalid_request"}
	}
	if c.receiptCode == "" {
		return nil, &OperatorRunError{Code: "receipt_required"}
	}
	s, _ := NewOperatorChallengeStore(objects, c.intent.EnvironmentID, c.intent.InstallationID)
	return &OperatorReceiptVerifier{c, s, operatorChannelReader(client, c.desired)}, nil
}
