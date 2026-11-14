package reconciliation

import (
	"bytes"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

// LiveBaselineSelector confirms a cumulative selection under current LIVE
// inputs. Bootstrap binds its backend, client and lease to the trusted location.
// It never discovers notices, reads credentials or sends notifications.
type LiveBaselineSelector struct {
	objects    BaselineSelectionObjects
	scope      discoveryScope
	diagnostic *DiagnosticReceiptVerifier
	operator   *OperatorReceiptVerifier
}

func NewLiveBaselineSelector(configBytes, catalogBytes []byte, location configuration.LedgerLocation, releaseSHA256 string, objects BaselineSelectionObjects, client *ocinotifications.Client) (*LiveBaselineSelector, error) {
	invalid := &LiveSelectionError{Code: "invalid_request"}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || (c.SchemaVersion != 6 && c.SchemaVersion != 7) {
		return nil, invalid
	}
	if c.Mode != "LIVE" {
		return nil, &LiveSelectionError{Code: "intent_required"}
	}
	d, err := NewDiagnosticReceiptVerifier(configBytes, catalogBytes, location, releaseSHA256, objects)
	if err != nil {
		return nil, invalid
	}
	o, err := NewOperatorReceiptVerifier(configBytes, catalogBytes, location, objects, client)
	if err != nil {
		return nil, invalid
	}
	return &LiveBaselineSelector{objects, baselineScope(c), d, o}, nil
}
