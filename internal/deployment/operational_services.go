package deployment

import (
	"context"
	"errors"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/reconciliation"
)

// Operation closures retain genuine coordinators; no decoded result grants authority.
type operations struct {
	initialize func(context.Context, *reconciliation.Lease) error
	prepare    func(context.Context, *reconciliation.Lease) (reconciliation.Baseline, error)
	diagnostic func(context.Context, *reconciliation.Lease) (reconciliation.DiagnosticResult, error)
	operator   func(context.Context, *reconciliation.Lease) (reconciliation.OperatorResult, error)
	selectLive func(context.Context, *reconciliation.Lease) (reconciliation.Baseline, error)
	process    func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error)
}

func composeOperations(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
	var result operations
	objects, ok := store.(operationalObjects)
	if !ok {
		return result, errors.New("invalid storage binding")
	}
	location := configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}
	mode := p.config.Mode
	if operation == "reconcile" && mode != "LIVE" && p.config.Recovery != nil {
		processor, err := reconciliation.NewMaintenanceProcessor(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, nil, objects, nil, nil, nil)
		if err != nil {
			return operations{}, err
		}
		result.process = processingOperation(processor)
	}
	if mode == "PAUSED" || mode == "TEST" && operation == "reconcile" {
		return result, nil
	}
	managed, ok := store.(*managedStore)
	if !ok || managed.principal == nil {
		return operations{}, errors.New("managed service binding required")
	}
	key := p.catalog.Regions[s.Region]
	var announcements reconciliation.AnnouncementSource
	if mode == "PREVIEW" || mode == "LIVE" && operation == "reconcile" {
		client, err := ociannouncements.New(s.Region, s.TenancyID, managed.principal)
		if err != nil {
			return operations{}, err
		}
		announcements = client
	}
	if mode == "PREVIEW" {
		preparation, err := reconciliation.NewBaselinePreparation(p.configBytes, p.catalogBytes, location, announcements, objects)
		if err != nil {
			return operations{}, err
		}
		result.initialize, result.prepare = preparation.Initialize, preparation.Prepare
		return result, nil
	}
	var channel *ocinotifications.Client
	if mode == "LIVE" || p.config.OperatorTest != nil {
		var err error
		channel, err = ocinotifications.New(s.Region, key, s.TenancyID, managed.principal)
		if err != nil {
			return operations{}, err
		}
	}
	if mode == "LIVE" && operation == "deployment-check" {
		selector, err := reconciliation.NewLiveBaselineSelector(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, objects, channel)
		if err != nil {
			return operations{}, err
		}
		result.selectLive = selector.Select
		return result, nil
	}
	vault, err := ocisecrets.New(s.Region, key, managed.principal)
	if err != nil {
		return operations{}, err
	}
	if mode == "TEST" {
		diagnostic, err := reconciliation.NewDiagnosticCoordinator(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, objects, vault)
		if err != nil {
			return operations{}, err
		}
		result.diagnostic = diagnostic.Run
		if p.config.OperatorTest != nil {
			operator, err := reconciliation.NewOperatorCoordinator(p.configBytes, p.catalogBytes, location, objects, channel)
			if err != nil {
				return operations{}, err
			}
			result.operator = operator.Run
		}
		return result, nil
	}
	mysql := make(map[string]*ocimysql.Client, len(p.config.RegionMap))
	for region, regionKey := range p.config.RegionMap {
		client, err := ocimysql.New(region, regionKey, s.TenancyID, managed.principal)
		if err != nil {
			return operations{}, err
		}
		mysql[region] = client
	}
	processor, err := reconciliation.NewMaintenanceProcessor(p.configBytes, p.catalogBytes, location, s.ReleaseSHA256, announcements, objects, channel, vault, mysql)
	if err != nil {
		return operations{}, err
	}
	result.process = processingOperation(processor)
	return result, nil
}

func processingOperation(processor *reconciliation.MaintenanceProcessor) func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
	return func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
		result, err := processor.Run(ctx, lease)
		return result.Snapshot(), err
	}
}
