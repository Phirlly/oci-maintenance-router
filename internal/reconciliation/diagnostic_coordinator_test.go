package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"oci-maintenance-router/internal/pagerduty"
)

func TestDiagnosticCoordinatorPersistsBeforeOneDispatch(t *testing.T) {
	f := coordinatorFixture(t)
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal("valid first diagnostic failed", err)
	}
	s := got.Record().Snapshot()
	if !got.Dispatched() || got.ObservedStatus() != 202 || s.State != DiagnosticAccepted || s.SecretVersion != 7 || s.OwnerToken != f.lease.record.OwnerToken || s.ClaimToken == s.OwnerToken || s.AcceptedAt.IsZero() {
		t.Fatal("first dispatch did not preserve confirmed acceptance")
	}
	a, _ := got.Record().Encode()
	b, _ := f.saved(t).Encode()
	if !bytes.Equal(a, b) || f.reads.Load() != 1 || f.sends.Load() != 1 || len(f.objects.noticeWrites()) != 0 || preparationCalls(f.objects, "LIST", "") != 0 {
		t.Fatal("wrong persisted result, effects or credential selection")
	}
	_, expiry := f.lease.record.times()
	if !s.ExpiresAt.Equal(expiry) {
		t.Fatal("claim did not retain lease expiry")
	}
	copy := s.PayloadWithoutRoutingKey
	clear(copy)
	if !bytes.Equal(got.Record().Snapshot().PayloadWithoutRoutingKey, f.saved(t).Snapshot().PayloadWithoutRoutingKey) {
		t.Fatal("result lost owned payload")
	}
	for range 2 {
		again, err := f.coordinator.Run(leaseBudget(t), f.lease)
		body, _ := again.Record().Encode()
		if err != nil || again.Dispatched() || again.ObservedStatus() != 0 || !bytes.Equal(a, body) || f.reads.Load() != 1 || f.sends.Load() != 1 {
			t.Fatal("repeat resent or changed accepted state", err)
		}
	}
}

func TestDiagnosticCoordinatorNeverRetriesTerminalOutcomes(t *testing.T) {
	for _, status := range []int{0, 200, 202, 429, 503, 600, 999} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := coordinatorFixture(t)
			f.coordinator.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
				f.sends.Add(1)
				return pagerduty.Response{Status: status, RetryAfter: time.Now().Add(time.Hour)}, errors.New("private provider data")
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			s := got.Record().Snapshot()
			wantStatus := status
			if status < 100 || status > 599 {
				wantStatus = 0
			}
			want := DiagnosticReview
			if status == 202 {
				want = DiagnosticAccepted
			}
			if !got.Dispatched() || got.ObservedStatus() != wantStatus || s.State != want || s.HTTPStatus != wantStatus {
				t.Fatal("incorrect observed/saved result")
			}
			again, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil || again.Dispatched() || f.sends.Load() != 1 || f.reads.Load() != 1 {
				t.Fatal("terminal test retried", err)
			}
		})
	}
}
