package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestProcessingPreparationIssueConfirmsLostCreateAndReplaceOnce(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "replace"}[replace], func(t *testing.T) {
			large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
			n := processingNotice(t, 1, processingDB, large)
			f := processingFixtureFor(t, n)
			if replace {
				f.run(t)
				at := time.Now().Add(time.Second)
				f.live.lease.manager.now = func() time.Time { return at }
				f.p.discovery.now = f.live.lease.manager.now
			}
			name := preparationIssueName(f.savedOrObserved(t, n))
			before := preparationCalls(f.live.objects, "GET", name)
			writes := 0
			f.live.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.name == name && (c.method == "CREATE" || c.method == "REPLACE") {
					writes++
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
			s := f.run(t)
			if !s.Completed || writes != 1 || preparationCalls(f.live.objects, "GET", name)-before != 2 || f.sent.Load() != 1 {
				t.Fatal("uncertain write not confirmed once", writes, f.sent.Load())
			}
		})
	}
}

func TestProcessingPreparationIssueDefiniteConflictStopsDependentPeer(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	name := preparationIssueName(f.savedOrObserved(t, n))
	f.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.name == name && c.method == "CREATE" {
			return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if e, ok := err.(*ProcessingError); !ok || e.Code != "issue_conflict" || e.WriteUncertain || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("conflict allowed dependent send", err)
	}
	if preparationCalls(f.live.objects, "GET", name) != 1 || preparationCalls(f.live.objects, "CREATE", name) != 1 {
		t.Fatal("definite conflict triggered confirmation/retry")
	}
}

func TestProcessingPreparationIssueUncertainConfirmationRejectsMissingOrChangedBytes(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
		n := processingNotice(t, 1, processingDB, large)
		f := processingFixtureFor(t, n)
		name := preparationIssueName(f.savedOrObserved(t, n))
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if c.name == name && c.method == "CREATE" {
				if corrupt {
					f.live.objects.seed(name, []byte("invalid"))
				} else {
					f.live.objects.mu.Lock()
					delete(f.live.objects.objects, name)
					f.live.objects.mu.Unlock()
				}
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if e, ok := err.(*ProcessingError); !ok || !e.WriteUncertain || r.Snapshot().Completed || f.sent.Load() != 0 {
			t.Fatal("unconfirmed sidecar allowed delivery", err)
		}
		if preparationCalls(f.live.objects, "CREATE", name) != 1 {
			t.Fatal("uncertain write replayed")
		}
	}
}
