package reconciliation

import (
	"bytes"
	"context"
	"slices"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocistorage"
)

// PreparationObjects provides bounded conditional storage and retained-history
// inspection. Bootstrap binds it and the lease to the same trusted location.
type PreparationObjects interface {
	ocistorage.NoticeObjects
	HistoryIsEmpty(context.Context, string) (bool, error)
}

// BaselinePreparation owns pinned PREVIEW intent. Preparing history never
// activates delivery, sends events or supplies fresh evidence for a covered scope.
type BaselinePreparation struct {
	discovery          *Discovery
	objects            PreparationObjects
	installation, mode string
	initialize         bool
	services           []string
	resourceServices   map[string][]string
}

func NewBaselinePreparation(configBytes, catalogBytes []byte, location configuration.LedgerLocation, announcements AnnouncementSource, objects PreparationObjects) (*BaselinePreparation, error) {
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, &PreparationError{Code: "invalid_request"}
	}
	config, err := configuration.Decode(configBytes, catalog)
	if err != nil || (config.SchemaVersion != 2 && config.SchemaVersion != 3 && config.SchemaVersion != 4 && config.SchemaVersion != 5 && config.SchemaVersion != 6 && config.SchemaVersion != 7 && config.SchemaVersion != 8 && config.SchemaVersion != 9) {
		return nil, &PreparationError{Code: "invalid_request"}
	}
	discovery, err := NewDiscovery(configBytes, catalogBytes, location, announcements, objects)
	if err != nil {
		return nil, &PreparationError{Code: "invalid_request"}
	}
	services := slices.Clone(config.MySQLServices)
	slices.Sort(services)
	selected := config.ResourceServices
	for _, labels := range selected {
		slices.Sort(labels)
	}
	return &BaselinePreparation{discovery: discovery, objects: objects, installation: config.InstallationID, mode: config.Mode, initialize: config.Preparation.Initialize, services: services, resourceServices: selected}, nil
}

var _ PreparationObjects = (*ocistorage.Client)(nil)
