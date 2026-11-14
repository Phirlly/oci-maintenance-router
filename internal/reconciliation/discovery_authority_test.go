package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
)

func TestDiscoveryRechecksAuthorityAfterCollectionAndEachWrite(t *testing.T) {
	for _, stage := range []string{"collection", "write", "final"} {
		for _, control := range []string{"lease", "config", "catalog"} {
			t.Run(stage+"/"+control, func(t *testing.T) {
				n := candidateNotice(t, 1, nil, true)
				inputs := []notices.Notice{n}
				if stage == "final" {
					inputs = nil
				}
				d, s, lease := discoveryFixture(t, inputs, nil)
				change := func() {
					switch control {
					case "lease":
						s.seed(workerLeaseName, lease.record.bytes())
					case "config":
						s.seed("configuration/router.json", []byte(candidateConfig+" "))
					case "catalog":
						s.seed("configuration/catalog.json", []byte(discoveryCatalog+" "))
					}
				}
				if stage == "collection" {
					a := d.collector.announcements.(announcementDetails)
					old := a.get
					a.get = func(ctx context.Context, id string) (notices.Notice, error) {
						result, err := old(ctx, id)
						change()
						return result, err
					}
					d.collector.announcements = a
				}
				controls := 0
				s.after = func(_ context.Context, call discoveryCall) error {
					if stage == "write" && call.method == "CREATE" {
						change()
					}
					// Empty runs guard before collection, after it, and immediately before
					// success. Change controls as the second guard finishes its last read.
					if call.name == "configuration/catalog.json" {
						controls++
						if stage == "final" && controls == 2 {
							change()
						}
					}
					return nil
				}
				got, err := d.Run(leaseBudget(t), lease)
				code := "configuration_changed"
				if control == "lease" {
					code = "ownership_lost"
				}
				discoveryFailure(t, got, err, code)
				want := 0
				if stage == "write" {
					want = 1
				}
				if len(s.noticeWrites()) != want {
					t.Fatal("incorrect work after lost authority")
				}
			})
		}
	}
}

func TestDiscoveryReserveIsCheckedAfterPacingAndBetweenControlReads(t *testing.T) {
	for _, stage := range []string{"pacing", "router-read"} {
		t.Run(stage, func(t *testing.T) {
			n := candidateNotice(t, 1, nil, true)
			d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
			var deadline time.Time
			exhaust := func() { lease.manager.now = func() time.Time { return deadline.Add(-5 * time.Second) } }
			s.before = func(ctx context.Context, call discoveryCall) error {
				if call.name == workerLeaseName {
					deadline, _ = ctx.Deadline()
				}
				if stage == "router-read" && call.name == "configuration/catalog.json" {
					t.Fatal("catalog dispatched without reserve")
				}
				if stage == "pacing" && call.method == "LIST" {
					t.Fatal("saved listing dispatched after pacing exhausted reserve")
				}
				return nil
			}
			s.after = func(_ context.Context, call discoveryCall) error {
				if stage == "router-read" && call.name == "configuration/router.json" {
					exhaust()
				}
				return nil
			}
			d.wait = func(context.Context, time.Duration) error { exhaust(); return nil }
			got, err := d.Run(leaseBudget(t), lease)
			discoveryFailure(t, got, err, "insufficient_time")
			if len(s.noticeWrites()) != 0 {
				t.Fatal("insufficient budget wrote state")
			}
		})
	}
}

func TestDiscoveryLifetimeDoesNotRestartAfterCollection(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil)
	var deadline time.Time
	checks := 0
	start := time.Now()
	s.before = func(ctx context.Context, call discoveryCall) error {
		if call.name == workerLeaseName {
			seen, _ := ctx.Deadline()
			if checks == 0 {
				deadline = seen
				if seen.After(start.Add(121*time.Second)) || seen.Before(start.Add(119*time.Second)) {
					t.Fatal("wrong shared discovery budget")
				}
			} else if seen != deadline {
				t.Fatal("discovery budget was reset")
			}
			checks++
		}
		return nil
	}
	a := d.collector.announcements.(announcementDetails)
	old := a.get
	a.get = func(ctx context.Context, id string) (notices.Notice, error) {
		seen, _ := ctx.Deadline()
		if seen != deadline {
			t.Fatal("collection got a new deadline")
		}
		return old(ctx, id)
	}
	d.collector.announcements = a
	if _, err := d.Run(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	if checks < 4 {
		t.Fatal("durability did not share authority context")
	}
}

func TestDiscoveryOriginalCancellationReachesWriteAndCannotConfirm(t *testing.T) {
	origin, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	n := candidateNotice(t, 1, nil, true)
	d, s, lease := discoveryFixture(t, []notices.Notice{n}, nil, origin)
	s.after = func(ctx context.Context, call discoveryCall) error {
		if call.method == "CREATE" {
			cancel()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				t.Fatal("original cancellation did not reach write")
			}
		}
		return nil
	}
	got, err := d.Run(leaseBudget(t), lease)
	discoveryFailure(t, got, err, "canceled")
	if len(s.noticeWrites()) != 1 {
		t.Fatal("write was retried")
	}
	for _, call := range s.calls {
		if call.method == "GET" && strings.HasPrefix(call.name, "notices/") {
			t.Fatal("canceled write was reread")
		}
	}
}
