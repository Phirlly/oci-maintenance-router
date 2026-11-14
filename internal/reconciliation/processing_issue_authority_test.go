package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

// Pure observation supplies only identity for locating the sidecar in hook setup.
func (f *processingFixture) savedOrObserved(t *testing.T, n notices.Notice) noticeledger.Record {
	t.Helper()
	r, err := f.p.observer.Observe(noticeledger.Record{}, n, true, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProcessingPreparationIssueChecksCurrentBeforeWriteAndConfirmation(t *testing.T) {
	for _, stage := range []string{"history", "write"} {
		t.Run(stage, func(t *testing.T) {
			large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
			n := processingNotice(t, 1, processingDB, large)
			f := processingFixtureFor(t, n)
			name := preparationIssueName(f.savedOrObserved(t, n))
			f.live.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.name == name && ((stage == "history" && c.method == "HISTORY") || (stage == "write" && c.method == "CREATE")) {
					f.live.objects.seed("configuration/router.json", []byte("changed"))
					if stage == "write" {
						return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
					}
				}
				return nil
			}
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			if err == nil || r.Snapshot().Completed || f.sent.Load() != 0 {
				t.Fatal("lost CURRENT allowed work", err)
			}
			wantWrites := 0
			if stage == "write" {
				wantWrites = 1
			}
			if preparationCalls(f.live.objects, "CREATE", name) != wantWrites || preparationCalls(f.live.objects, "GET", name) != 1 {
				t.Fatal("authority loss allowed write/confirmation")
			}
		})
	}
}

func TestProcessingPreparationIssueCancellationDoesNotStartDetachedConfirmation(t *testing.T) {
	large := "ocid1.mysqldbsystem.oc1.iad." + strings.Repeat("x", 40000)
	n := processingNotice(t, 1, processingDB, large)
	f := processingFixtureFor(t, n)
	name := preparationIssueName(f.savedOrObserved(t, n))
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == name && c.method == "CREATE" {
			cancel()
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	r, err := f.p.Run(ctx, f.live.lease)
	if e, ok := err.(*ProcessingError); !ok || e.Code != "canceled" || r.Snapshot().Completed || f.sent.Load() != 0 {
		t.Fatal("cancellation allowed work", err)
	}
	if preparationCalls(f.live.objects, "GET", name) != 1 || preparationCalls(f.live.objects, "CREATE", name) != 1 {
		t.Fatal("detached confirmation or replay")
	}
}
