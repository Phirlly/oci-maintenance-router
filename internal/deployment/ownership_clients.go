package deployment

import (
	"errors"

	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ociownership"
	"oci-maintenance-router/internal/reconciliation"
)

// Normal processing and recovery review use the same closed regional factories.
// These constructors bind identities only; they never fetch a credential/resource.
func composeOwnership(c configuration.Config, tenancy string, provider common.KeyProvider) (reconciliation.OwnershipClients, error) {
	result := reconciliation.OwnershipClients{MySQL: map[string]*ociownership.Client{}, Compute: map[string]*ociownership.Client{}}
	kinds := []string{"mysqldbsystem"}
	if c.SchemaVersion == 9 {
		kinds = c.ResourceTypes
	}
	for _, kind := range kinds {
		makeClient, output := ociownership.NewMySQL, result.MySQL
		if kind == "instance" {
			makeClient, output = ociownership.NewCompute, result.Compute
		} else if kind != "mysqldbsystem" {
			return reconciliation.OwnershipClients{}, errors.New("unsupported ownership type")
		}
		for region, key := range c.RegionMap {
			client, err := makeClient(region, key, tenancy, provider)
			if err != nil {
				return reconciliation.OwnershipClients{}, err
			}
			output[region] = client
		}
	}
	return result, nil
}
