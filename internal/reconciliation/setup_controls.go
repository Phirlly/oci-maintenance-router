package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"oci-maintenance-router/internal/configuration"
)

type setupControls struct {
	config               configuration.Config
	environment, catalog string
	current              currentControls
	desired              OperatorChannel
}

func readSetupControls(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects setupCurrentObjects) (setupControls, error) {
	invalid := &SetupError{Code: "invalid_request"}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return setupControls{}, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || c.SchemaVersion != 8 || c.OperatorChannel == nil {
		return setupControls{}, invalid
	}
	env, err := c.EnvironmentID(location)
	if err != nil {
		return setupControls{}, invalid
	}
	if _, err = newSetupCurrentStore(objects, env, c.InstallationID); err != nil {
		return setupControls{}, invalid
	}
	hash := sha256.Sum256(catalogBytes)
	return setupControls{c, env, hex.EncodeToString(hash[:]), currentControls{objects, sha256.Sum256(configBytes), hash}, OperatorChannel{c.OperatorTopicID, c.OperatorChannel.SubscriptionID, "EMAIL", c.OperatorChannel.Endpoint}}, nil
}

func (c setupControls) requested(release string) (DiagnosticIntent, OperatorIntent) {
	d, o := setupRequestIDs(c.environment, c.config.InstallationID, c.config.Setup.Attempt)
	diagnostic := DiagnosticIntent{c.environment, c.config.InstallationID, d, release, c.catalog, c.config.PagerDutyRegion, c.config.Setup.SecretID, c.config.Setup.NonProduction}
	operator, _ := NewOperatorIntent(c.environment, c.config.InstallationID, o, c.desired)
	return diagnostic, operator
}

func (c setupControls) diagnosticInput(intent DiagnosticIntent, code string) diagnosticControls {
	return diagnosticControls{intent, c.current, 8, c.config.Mode, code}
}

func (c setupControls) operatorInput(intent OperatorIntent, code string) operatorControls {
	return operatorControls{intent, c.desired, c.current, c.config.Mode, code}
}
