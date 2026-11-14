package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoveryConstructorRejectsUnboundInputs(t *testing.T) {
	a := announcementDetails{}
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	for _, tt := range []struct {
		config, catalog []byte
		source          AnnouncementSource
		objects         ocistorage.NoticeObjects
	}{
		{nil, []byte(discoveryCatalog), a, objects},
		{[]byte(candidateConfig), []byte(`{"regions":{},"mysqlServices":[]}`), a, objects},
		{[]byte(candidateConfig), []byte(discoveryCatalog), nil, objects},
		{[]byte(candidateConfig), []byte(discoveryCatalog), (*announcementDetails)(nil), objects},
		{[]byte(candidateConfig), []byte(discoveryCatalog), a, nil},
		{[]byte(candidateConfig), []byte(discoveryCatalog), a, (*discoveryObjects)(nil)},
	} {
		got, err := NewDiscovery(tt.config, tt.catalog, candidateLocation(), tt.source, tt.objects)
		if err == nil || got != nil {
			t.Fatal("unbound discovery accepted")
		}
	}
	got, err := NewDiscovery([]byte(candidateConfig), []byte(discoveryCatalog), configuration.LedgerLocation{}, a, objects)
	if err == nil || got != nil {
		t.Fatal("unbound ledger location accepted")
	}
}

func TestDiscoveryOwnsBothExactConfigurationInputs(t *testing.T) {
	d, objects, lease := discoveryFixture(t, nil, nil)
	config, catalog := []byte(candidateConfig), []byte(discoveryCatalog)
	owned, err := NewDiscovery(config, catalog, candidateLocation(), d.collector.announcements, objects)
	if err != nil {
		t.Fatal(err)
	}
	owned.wait = d.wait
	owned.now = d.now
	clear(config)
	clear(catalog)
	if _, err := owned.Run(leaseBudget(t), lease); err != nil {
		t.Fatal("caller buffers changed retained settings", err)
	}
}

func TestDiscoveryRejectsChangedOrUnreadableCurrentControls(t *testing.T) {
	for _, name := range []string{"configuration/router.json", "configuration/catalog.json"} {
		for _, scenario := range []string{"wire-change", "missing", "failed"} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				n := candidateNotice(t, 1, nil, true)
				d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
				code := "configuration_changed"
				if scenario == "wire-change" {
					s.seed(name, append(s.objects[name].Body, ' '))
				}
				if scenario == "missing" {
					delete(s.objects, name)
					code = "configuration_read"
				}
				if scenario == "failed" {
					code = "configuration_read"
					s.before = func(_ context.Context, call discoveryCall) error {
						if call.name == name {
							return errors.New("private token")
						}
						return nil
					}
				}
				got, err := d.Run(leaseBudget(t), lease)
				discoveryFailure(t, got, err, code)
				if strings.Contains(err.Error(), "private") || len(s.noticeWrites()) != 0 {
					t.Fatal("failed controls leaked detail or allowed writes")
				}
			})
		}
	}
}

func TestDiscoveryRequiresValidLeaseAndCallerLifetimeBeforeIO(t *testing.T) {
	d, s, lease := discoveryFixture(t, nil, nil)
	s.before = func(context.Context, discoveryCall) error { t.Fatal("invalid authority reached storage"); return nil }
	for _, tt := range []struct {
		ctx   context.Context
		lease *Lease
		code  string
	}{
		{nil, lease, "invalid_request"}, {context.Background(), lease, "deadline_required"}, {leaseBudget(t), nil, "invalid_request"},
	} {
		got, err := d.Run(tt.ctx, tt.lease)
		discoveryFailure(t, got, err, tt.code)
	}
	ctx, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	got, err := d.Run(ctx, lease)
	discoveryFailure(t, got, err, "canceled")
	ctx, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	got, err = d.Run(ctx, lease)
	discoveryFailure(t, got, err, "deadline_exceeded")
	other := *lease
	manager := *lease.manager
	other.manager = &manager
	manager.environmentID = strings.Repeat("f", 64)
	got, err = d.Run(leaseBudget(t), &other)
	discoveryFailure(t, got, err, "environment_mismatch")
}

func TestDiscoveryOriginalCancellationReachesCollectionIO(t *testing.T) {
	origin, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil, origin)
	a := d.collector.announcements.(announcementDetails)
	a.get = func(ctx context.Context, _ string) (notices.Notice, error) {
		cancel()
		select {
		case <-ctx.Done():
			return notices.Notice{}, ctx.Err()
		case <-time.After(time.Second):
			t.Fatal("original lease cancellation did not reach source read")
			return notices.Notice{}, nil
		}
	}
	d.collector.announcements = a
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "canceled")
	if len(s.noticeWrites()) != 0 {
		t.Fatal("canceled collection wrote state")
	}
}
