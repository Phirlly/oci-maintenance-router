package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationHistoryBoundaryCannotForgeDiagnostics(t *testing.T) {
	for _, err := range []error{errors.New("private detail"), &PreparationError{Code: "private detail", WriteUncertain: true}, &DiscoveryError{Code: "private detail"}, fmt.Errorf("wrapped: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &ocistorage.Error{Code: "private detail", Status: 503}} {
		p, s, l := preparationFixture(t, nil)
		s.history = func(context.Context, string) (bool, error) { return true, err }
		failure := p.Initialize(leaseBudget(t), l)
		d := preparationFailed(t, Baseline{}, failure, "history_read")
		if strings.Contains(fmt.Sprintf("%+v", d), "private") || d.WriteUncertain {
			t.Fatal("backend diagnostic escaped")
		}
		want := 0
		if e, ok := err.(*ocistorage.Error); ok {
			want = e.Status
		}
		if d.HTTPStatus != want {
			t.Fatal("status boundary changed")
		}
	}
}

func TestPreparationPartialDiscoveryNeverCompletesHistory(t *testing.T) {
	n1, n2 := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	p, s, l := preparationFixture(t, []notices.Notice{n1, n2})
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	s.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "CREATE" && c.name == noticeName(n2.Key().NoticeID) {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	b, err := p.Prepare(leaseBudget(t), l)
	if err == nil || b.ID() != "" {
		t.Fatal("partial durable discovery completed baseline")
	}
	if _, ok := s.objects[noticeName(n1.Key().NoticeID)]; !ok {
		t.Fatal("confirmed earlier observation was lost")
	}
	if preparationHead(t, s, l).BaselineID() != "" {
		t.Fatal("partial pass advanced history")
	}
	for name := range s.objects {
		if strings.HasPrefix(name, "baselines/") {
			t.Fatal("partial pass wrote baseline")
		}
	}
}
