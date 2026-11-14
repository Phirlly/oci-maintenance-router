package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocistorage"
)

// Discovery coordinates durable observations; it never authorizes delivery.
type Discovery struct {
	collector               *Collector
	objects                 ocistorage.NoticeObjects
	configHash, catalogHash [32]byte
	scope                   discoveryScope
	wait                    func(context.Context, time.Duration) error
	now                     func() time.Time
}

// DiscoveryResult is confirmed discovery evidence, not a baseline or permission
// to send. Only a successful complete Run can construct a nonzero result.
type DiscoveryResult struct {
	environmentID string
	view          CandidateView
	availability  SourceAvailability
	completedAt   time.Time
	scope         discoveryScope
}

func (r DiscoveryResult) EnvironmentID() string            { return r.environmentID }
func (r DiscoveryResult) Candidates() CandidateView        { return r.view }
func (r DiscoveryResult) Availability() SourceAvailability { return r.availability }
func (r DiscoveryResult) CompletedAt() time.Time           { return r.completedAt }

func NewDiscovery(configBytes, catalogBytes []byte, location configuration.LedgerLocation, announcements AnnouncementSource, objects ocistorage.NoticeObjects) (*Discovery, error) {
	if announcements == nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	v := reflect.ValueOf(announcements)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil, &DiscoveryError{Code: "invalid_request"}
		}
	}
	configBytes, catalogBytes = bytes.Clone(configBytes), bytes.Clone(catalogBytes)
	catalog, err := configuration.DecodeCatalog(catalogBytes)
	if err != nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	config, err := configuration.Decode(configBytes, catalog)
	if err != nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	environment, err := config.EnvironmentID(location)
	if err != nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	store, err := ocistorage.NewNoticeStore(objects, environment)
	if err != nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	collector, err := NewCollector(configBytes, catalog, location, announcements, store)
	if err != nil {
		return nil, &DiscoveryError{Code: "invalid_request"}
	}
	return &Discovery{collector: collector, objects: objects, configHash: sha256.Sum256(configBytes), catalogHash: sha256.Sum256(catalogBytes), scope: baselineScope(config), wait: pause, now: time.Now}, nil
}

// Run durably records a complete bounded discovery. Partial confirmed writes
// survive failure, but no failure returns completion evidence. The caller owns
// lease release. Adapters must be bound to the trusted location by bootstrap.
func (d *Discovery) Run(ctx context.Context, lease *Lease) (DiscoveryResult, error) {
	if d == nil || d.collector == nil || d.objects == nil || d.wait == nil || d.now == nil || !lease.usable() {
		return DiscoveryResult{}, &DiscoveryError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return DiscoveryResult{}, discoveryFailureFor(ctx, "invalid_request", err)
	}
	if lease.manager.environmentID != d.collector.environmentID {
		return DiscoveryResult{}, &DiscoveryError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	ctx, finish, err := discoveryContext(ctx)
	if err != nil {
		return DiscoveryResult{}, discoveryFailureFor(ctx, "invalid_request", err)
	}
	defer finish()
	run := &discoveryRun{owner: d, ctx: ctx, lease: lease}
	// Each run owns its handles and reserve checks; concurrent runs share no
	// mutable coordinator state. Store validity was established by construction.
	run.store, _ = ocistorage.NewNoticeStore(discoveryStorage{objects: d.objects, reserve: run.reserve}, d.collector.environmentID)
	result, failure := run.run()
	if failure != nil {
		return DiscoveryResult{}, discoveryFailureFor(ctx, "discovery_failed", failure)
	}
	return result, nil
}

type discoveryRun struct {
	owner *Discovery
	ctx   context.Context
	lease *Lease
	store *ocistorage.NoticeStore
}

func (r *discoveryRun) run() (DiscoveryResult, error) {
	if err := r.guard(); err != nil {
		return DiscoveryResult{}, err
	}
	collector := *r.owner.collector
	collector.saved = r.store
	collection, err := collector.collectBounded(r.ctx, r.owner.wait, func() error {
		if err := r.reserve(); err != nil {
			return &Error{Code: Code(err.(*DiscoveryError).Code)}
		}
		return nil
	})
	if err != nil {
		return DiscoveryResult{}, discoveryFailureFor(r.ctx, "collection_failed", err)
	}
	if err := r.guard(); err != nil {
		return DiscoveryResult{}, err
	}
	work, err := discoveryWorks(collection)
	if err != nil {
		return DiscoveryResult{}, err
	}
	for _, item := range work {
		if err := r.persist(item); err != nil {
			return DiscoveryResult{}, err
		}
	}
	if err := r.guard(); err != nil {
		return DiscoveryResult{}, err
	}
	at := r.owner.now().UTC().Truncate(time.Millisecond)
	if !leaseWallTime(at) {
		return DiscoveryResult{}, &DiscoveryError{Code: "clock_invalid"}
	}
	return DiscoveryResult{environmentID: r.owner.collector.environmentID, view: collection.Candidates, availability: collection.Availability, completedAt: at, scope: r.owner.scope}, nil
}
