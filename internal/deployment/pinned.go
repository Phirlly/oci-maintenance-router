package deployment

import (
	"context"
	"errors"

	"oci-maintenance-router/internal/configuration"
)

type pinnedInputs struct {
	configBytes, catalogBytes []byte
	config                    configuration.Config
	catalog                   configuration.Catalog
	environment, generation   string
}

func loadPinned(ctx context.Context, s Settings, store Store) (pinnedInputs, Result) {
	var p pinnedInputs
	if ctx.Err() != nil {
		return p, failed("deadline")
	}
	var err error
	p.catalogBytes, err = readPinned(ctx, store, "configuration/catalog.json", s.CatalogVersion, s.CatalogSHA256)
	if err != nil {
		return pinnedInputs{}, failed("catalog_read")
	}
	p.catalog, err = configuration.DecodeCatalog(p.catalogBytes)
	if err != nil {
		return pinnedInputs{}, failed("catalog_invalid")
	}
	p.configBytes, err = readPinned(ctx, store, "configuration/router.json", s.ConfigurationVersion, s.ConfigurationSHA256)
	if err != nil {
		return pinnedInputs{}, failed("configuration_read")
	}
	p.config, err = configuration.Decode(p.configBytes, p.catalog)
	if err != nil {
		result := failed("configuration_invalid")
		var detail *configuration.Error
		if errors.As(err, &detail) {
			result.Field = detail.Field
		}
		return pinnedInputs{}, result
	}
	if p.config.TenancyID != s.TenancyID || p.config.DeploymentRegion != s.Region {
		return pinnedInputs{}, failed("configuration_identity")
	}
	p.environment, err = p.config.EnvironmentID(configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket})
	if err != nil {
		return pinnedInputs{}, failed("storage_configuration")
	}
	p.generation, err = p.config.RoutingGeneration()
	if err != nil {
		return pinnedInputs{}, failed("configuration_invalid")
	}
	if ctx.Err() != nil {
		return pinnedInputs{}, failed("deadline")
	}
	return p, Result{}
}
