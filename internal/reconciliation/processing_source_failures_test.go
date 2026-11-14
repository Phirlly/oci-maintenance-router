package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceLateSourceUnavailableIsPersistedBeforeSkipping(t *testing.T) {
	for _, status := range []int{401, 403, 404, 408, 429, 500, 502, 503, 504} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		get := f.source.get
		f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
			if f.fetched.Load() >= 2 {
				return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: status}
			}
			return get(ctx, id)
		}
		f.p.discovery.collector.announcements = f.source
		s := f.run(t)
		saved := f.saved(t, n.Key().NoticeID).Snapshot()
		if s.Accepted != 0 || len(s.Issues) != 1 || f.sent.Load() != 0 || saved.SourceOutcome.State != noticeledger.SourceUnavailable || saved.SourceOutcome.HTTPStatus != status {
			t.Fatal("late source failure not retained")
		}
	}
}

func TestMaintenanceCannotReportUnavailabilityWhenItsTransitionFails(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	get := f.source.get
	f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
		if f.fetched.Load() < 2 {
			return get(ctx, id)
		}
		f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}
	f.p.discovery.collector.announcements = f.source
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || len(r.Snapshot().Issues) != 0 || f.sent.Load() != 0 {
		t.Fatal("failed unavailability transition reported completion", err)
	}
	if f.saved(t, n.Key().NoticeID).Snapshot().SourceOutcome.State == noticeledger.SourceUnavailable {
		t.Fatal("failed transition changed durable source outcome")
	}
}

func TestMaintenanceLateInvalidSourceCannotForgeControlledErrors(t *testing.T) {
	for _, kind := range []string{"raw", "forged", "wrong-id", "result-and-error", "empty"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		get := f.source.get
		f.source.get = func(ctx context.Context, id string) (notices.Notice, error) {
			if f.fetched.Load() < 2 {
				return get(ctx, id)
			}
			switch kind {
			case "raw":
				return notices.Notice{}, errors.New("private-provider-text")
			case "forged":
				return notices.Notice{}, &ProcessingError{Code: "private-provider-text"}
			case "wrong-id":
				return processingNotice(t, 2, processingDB), nil
			case "result-and-error":
				return n, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
			default:
				return notices.Notice{}, nil
			}
		}
		f.p.discovery.collector.announcements = f.source
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || strings.Contains(err.Error(), "private") || f.sent.Load() != 0 {
			t.Fatal("invalid source permitted work or leaked")
		}
	}
}

func TestMaintenanceUnsupportedNoticeProducesReviewWithoutCredentials(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	f := processingFixtureFor(t, n)
	s := f.run(t)
	if !s.Completed || len(s.Issues) != 1 || s.Issues[0].Code != "unsupported_type" || f.current.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("unsupported notice hidden or delivered")
	}
}

func TestMaintenanceAuthorityAndClockChangesDuringOwnershipStopPreparation(t *testing.T) {
	for _, kind := range []string{"configuration", "clock", "storage", "cancel"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		ctx, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		lookup := f.p.mysql["us-ashburn-1"]
		f.p.mysql["us-ashburn-1"] = func(c context.Context, id string) (routing.Metadata, error) {
			m, e := lookup(c, id)
			switch kind {
			case "configuration":
				f.live.objects.seed("configuration/catalog.json", []byte("changed"))
			case "clock":
				f.live.lease.manager.now = func() time.Time { return time.Unix(1, 0) }
			case "storage":
				f.live.objects.before = func(_ context.Context, call discoveryCall) error {
					if call.method == "REPLACE" && call.name == noticeName(n.Key().NoticeID) {
						return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
					}
					return nil
				}
			case "cancel":
				cancel()
			}
			return m, e
		}
		r, err := f.p.Run(ctx, f.live.lease)
		if err == nil || r.Snapshot().Completed || f.current.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("changed authority/clock performed preparation")
		}
	}
}
