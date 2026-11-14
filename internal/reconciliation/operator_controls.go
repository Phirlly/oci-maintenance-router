package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"oci-maintenance-router/internal/configuration"
)

// Shared pinned derivation keeps publication and receipt attestation on the same
// identity; receipt input and routing changes remain outside the operator intent.
type operatorControls struct {
	intent            OperatorIntent
	desired           OperatorChannel
	current           currentControls
	mode, receiptCode string
}

func readOperatorControls(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects OperatorChallengeObjects) (operatorControls, error) {
	invalid := &OperatorRunError{Code: "invalid_request"}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return operatorControls{}, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || (c.SchemaVersion != 5 && c.SchemaVersion != 6) {
		return operatorControls{}, invalid
	}
	if c.OperatorTest == nil {
		return operatorControls{}, &OperatorRunError{Code: "intent_required"}
	}
	environment, err := c.EnvironmentID(location)
	if err != nil {
		return operatorControls{}, invalid
	}
	if _, err := NewOperatorChallengeStore(objects, environment, c.InstallationID); err != nil {
		return operatorControls{}, invalid
	}
	desired := OperatorChannel{c.OperatorTopicID, c.OperatorTest.SubscriptionID, "EMAIL", c.OperatorTest.Endpoint}
	intent, _ := NewOperatorIntent(environment, c.InstallationID, c.OperatorTest.RequestID, desired) // Validated by configuration.
	return operatorControls{intent, desired, currentControls{objects, sha256.Sum256(configBytes), sha256.Sum256(catalogBytes)}, c.Mode, c.OperatorTest.ReceiptCode}, nil
}
