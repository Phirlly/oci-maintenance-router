package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
)

func TestMaintenanceDistinguishesKnownUnsentFromAmbiguousFailures(t *testing.T) {
	for _, unsent := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.p.senders["US"] = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
			f.sent.Add(1)
			return pagerduty.Response{}, &pagerduty.Error{Code: pagerduty.Transport, MayHaveSent: !unsent}
		}
		s := f.run(t)
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if unsent {
			if len(s.Attempts) != 1 || s.Attempts[0].MayHaveSent || b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonOperator || b.Ambiguous {
				t.Fatal("known no-dispatch became ambiguous/retryable")
			}
		} else if len(s.Attempts) != 3 || !s.Attempts[0].MayHaveSent || b.State != delivery.RetryWait || !b.Ambiguous {
			t.Fatal("uncertain send not retained")
		}
	}
}

func TestMaintenanceUnexpectedStatusIsPermanentAndRawFailureIsPrivate(t *testing.T) {
	for _, status := range []int{301, 400, 401, 403, 204} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.p.senders["US"] = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
			f.sent.Add(1)
			return pagerduty.Response{Status: status}, nil
		}
		s := f.run(t)
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if len(s.Attempts) != 1 || s.Accepted != 0 || b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonPermanent {
			t.Fatal("permanent response retried")
		}
	}
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.p.senders["US"] = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
		return pagerduty.Response{}, errors.New("private-provider-content")
	}
	s := f.run(t)
	if len(s.Attempts) != 3 || !s.Attempts[0].MayHaveSent {
		t.Fatal("raw failure was trusted as no dispatch")
	}
}
