package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/notices"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceFailedPacingStopsEveryBusinessStage(t *testing.T) {
	for _, stopAt := range []int{1, 2, 3, 5, 6} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		waits := 0
		f.p.wait = func(_ context.Context, delay time.Duration) error {
			if delay != 100*time.Millisecond {
				t.Fatal("wrong service pacing")
			}
			waits++
			if waits == stopAt {
				return errors.New("private wait detail")
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 || waits != stopAt || strings.Contains(err.Error(), "private") {
			t.Fatal("failed pacing performed later work", stopAt, waits, err)
		}
	}
}

func TestMaintenanceRechecksControlsAfterFinalDispatchPacing(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	waits := 0
	f.p.wait = func(context.Context, time.Duration) error {
		waits++
		if waits == 6 {
			f.live.objects.seed("configuration/router.json", []byte("changed"))
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || len(r.Snapshot().Attempts) != 0 || f.sent.Load() != 0 || waits != 6 {
		t.Fatal("controls not rechecked after pacing", err)
	}
}

func TestMaintenanceCancellationDuringSourceAndCredentialReadsStopsWork(t *testing.T) {
	for _, stage := range []string{"source", "current", "version"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		ctx, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		switch stage {
		case "source":
			get := f.source.get
			f.source.get = func(c context.Context, id string) (notices.Notice, error) {
				n, e := get(c, id)
				if f.fetched.Load() == 3 {
					cancel()
				}
				return n, e
			}
			f.p.discovery.collector.announcements = f.source
		case "current":
			current := f.p.current
			f.p.current = func(c context.Context, id string) (maintenanceCredential, error) {
				v, e := current(c, id)
				cancel()
				return v, e
			}
		case "version":
			version := f.p.version
			f.p.version = func(c context.Context, id string, n int64) (maintenanceCredential, error) {
				v, e := version(c, id, n)
				cancel()
				return v, e
			}
		}
		r, err := f.p.Run(ctx, f.live.lease)
		if !errors.Is(err, context.Canceled) || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("canceled read continued", stage, err)
		}
	}
}

func TestMaintenanceOriginalLeaseCancellationCannotBeReplaced(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	if err := f.live.lease.Release(leaseBudget(t)); err != nil {
		t.Fatal(err)
	}
	origin, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.live.lease = acquireLease(t, f.live.lease.manager, origin)
	get := f.source.get
	f.source.get = func(c context.Context, id string) (notices.Notice, error) {
		n, e := get(c, id)
		if f.fetched.Load() == 3 {
			cancel()
		}
		return n, e
	}
	f.p.discovery.collector.announcements = f.source
	independent := leaseBudget(t)
	r, err := f.p.Run(independent, f.live.lease)
	if !errors.Is(err, context.Canceled) || independent.Err() != nil || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("new caller revived canceled lease origin", err)
	}
}
