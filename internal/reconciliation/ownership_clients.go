package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ociownership"
	"oci-maintenance-router/internal/routing"
)

// OwnershipClients contains the fixed regional readers supplied by bootstrap.
// Only selected types are required or retained; construction performs no I/O.
type OwnershipClients struct {
	MySQL, Compute map[string]*ociownership.Client
}

type ownershipReaders struct {
	mysql, compute map[string]func(context.Context, string) (routing.Metadata, error)
}

func bindOwnership(c configuration.Config, clients OwnershipClients) (ownershipReaders, bool) {
	r := ownershipReaders{mysql: map[string]func(context.Context, string) (routing.Metadata, error){}, compute: map[string]func(context.Context, string) (routing.Metadata, error){}}
	kinds := []string{"mysqldbsystem"}
	if c.SchemaVersion == 9 {
		kinds = c.ResourceTypes
	}
	for _, kind := range kinds {
		input, output := clients.MySQL, r.mysql
		if kind == "instance" {
			input, output = clients.Compute, r.compute
		}
		for region, key := range c.RegionMap {
			client := input[region]
			if !client.BoundTo(kind, region, key, c.TenancyID) {
				return ownershipReaders{}, false
			}
			output[region] = client.Get
		}
	}
	return r, true
}

func (r ownershipReaders) reader(target routing.Target) func(context.Context, string) (routing.Metadata, error) {
	switch routing.ResourceType(target.ResourceID) {
	case "mysqldbsystem":
		return r.mysql[target.Region]
	case "instance":
		return r.compute[target.Region]
	}
	return nil
}

func ownershipResult(metadata routing.Metadata, failure error) routing.Lookup {
	if failure == nil {
		return routing.Lookup{State: routing.Found, Metadata: metadata}
	}
	lookup := routing.Lookup{State: routing.Unavailable}
	if e, ok := failure.(*ociownership.Error); ok && e != nil && e.Code == ociownership.Service {
		if e.Status == 401 || e.Status == 403 {
			lookup.State = routing.Denied
		}
		if e.Status == 404 {
			lookup.State = routing.NotFound
		}
	}
	return lookup
}
