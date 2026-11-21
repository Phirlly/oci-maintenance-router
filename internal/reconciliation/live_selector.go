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
	setup      *setupReadiness
	release    string
}

func NewLiveBaselineSelector(configBytes, catalogBytes []byte, location configuration.LedgerLocation, releaseSHA256 string, objects BaselineSelectionObjects, client *ocinotifications.Client) (*LiveBaselineSelector, error) {
	invalid := &LiveSelectionError{Code: "invalid_request"}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || (c.SchemaVersion != 6 && c.SchemaVersion != 7 && c.SchemaVersion != 8 && c.SchemaVersion != 9) {
		return nil, invalid
	}
	if c.Mode != "LIVE" {
		return nil, &LiveSelectionError{Code: "intent_required"}
	}
	if c.SchemaVersion == 8 || c.SchemaVersion == 9 {
		v, err := newSetupReadiness(configBytes, catalogBytes, location, objects, client)
		if err != nil || !leaseDigest.MatchString(releaseSHA256) {
			return nil, invalid
		}
		return &LiveBaselineSelector{objects: objects, scope: baselineScope(c), setup: v, release: releaseSHA256}, nil
	}
	d, err := NewDiagnosticReceiptVerifier(configBytes, catalogBytes, location, releaseSHA256, objects)
	if err != nil {
		return nil, invalid
	}
	o, err := NewOperatorReceiptVerifier(configBytes, catalogBytes, location, objects, client)
	if err != nil {
		return nil, invalid
	}
	return &LiveBaselineSelector{objects: objects, scope: baselineScope(c), diagnostic: d, operator: o}, nil
}

func (s *LiveBaselineSelector) controls() (string, string, currentControls) {
	if s.setup != nil {
		return s.setup.input.environment, s.setup.input.config.InstallationID, s.setup.input.current
	}
	i := s.diagnostic.input
	return i.intent.EnvironmentID, i.intent.InstallationID, i.current
}
