package reconciliation

import (
	"bytes"
	"context"
	"math/rand/v2"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocimysql"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocisecrets"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
	"time"
)

// MaintenanceObjects binds notice and activation storage to one trusted location.
type MaintenanceObjects interface {
	ocistorage.NoticeObjects
	BaselineSelectionObjects
}

type maintenanceCredential interface {
	Version() int64
	Bytes() []byte
}

// MaintenanceProcessor composes automatic LIVE processing. Bootstrap owns the
// trusted tenancy/region/client bindings; the caller owns lease release.
type MaintenanceProcessor struct {
	objects   MaintenanceObjects
	discovery *Discovery
	selector  *LiveBaselineSelector
	observer  *noticeledger.Observer
	policy    *routing.Policy
	builder   *delivery.Builder
	mysql     map[string]func(context.Context, string) (routing.Metadata, error)
	current   func(context.Context, string) (maintenanceCredential, error)
	version   func(context.Context, string, int64) (maintenanceCredential, error)
	senders   map[string]func(context.Context, []byte, []byte) (pagerduty.Response, error)
	wait      func(context.Context, time.Duration) error
	jitter    func(time.Duration) time.Duration
	token     func() string
}

func NewMaintenanceProcessor(configBytes, catalogBytes []byte, location configuration.LedgerLocation, releaseSHA256 string, announcements AnnouncementSource, objects MaintenanceObjects, notifications *ocinotifications.Client, vault *ocisecrets.Client, mysql map[string]*ocimysql.Client) (*MaintenanceProcessor, error) {
	invalid := &ProcessingError{Code: "invalid_request"}
	if vault == nil {
		return nil, invalid
	}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, invalid
	}
	c, err := configuration.Decode(configBytes, catalog)
	if err != nil || c.SchemaVersion != 6 {
		return nil, invalid
	}
	if c.Mode != "LIVE" {
		return nil, &ProcessingError{Code: "intent_required"}
	}
	selector, err := NewLiveBaselineSelector(configBytes, catalogBytes, location, releaseSHA256, objects, notifications)
	if err != nil {
		return nil, invalid
	}
	discovery, err := NewDiscovery(configBytes, catalogBytes, location, announcements, objects)
	if err != nil {
		return nil, invalid
	}
	lookups := make(map[string]func(context.Context, string) (routing.Metadata, error), len(c.RegionMap))
	for region := range c.RegionMap {
		client := mysql[region]
		if client == nil {
			return nil, invalid
		}
		lookups[region] = client.Get
	}
	// The shared decoded inputs and trusted location were validated above.
	observer, _ := noticeledger.New(configBytes, catalog, location)
	policy, _ := routing.New(configBytes, catalog)
	builder, _ := delivery.New(configBytes, catalog, location)
	senders := map[string]func(context.Context, []byte, []byte) (pagerduty.Response, error){}
	for _, region := range []string{"US", "EU"} {
		client, _ := pagerduty.New(region)
		senders[region] = client.Send
	}
	return &MaintenanceProcessor{objects: objects, discovery: discovery, selector: selector,
		observer: observer, policy: policy, builder: builder, mysql: lookups,
		current: func(ctx context.Context, id string) (maintenanceCredential, error) { return vault.GetCurrent(ctx, id) },
		version: func(ctx context.Context, id string, version int64) (maintenanceCredential, error) {
			return vault.GetVersion(ctx, id, version)
		},
		senders: senders, wait: pause, jitter: func(limit time.Duration) time.Duration { return time.Duration(rand.Int64N(int64(limit) + 1)) }, token: newLeaseToken}, nil
}
