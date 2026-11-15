package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestOperatorCoordinatorPersistsBeforeOnePublication(t *testing.T) {
	for _, status := range []int{200, 202} {
		f := operatorStatusFixture(t, status)
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if err != nil {
			t.Fatal("valid first publication failed", err)
		}
		s := got.Record().Snapshot()
		_, expiry := f.lease.record.times()
		if !got.Dispatched() || got.ObservedStatus() != status || got.ObservedMessageID() != "message-1" || s.State != OperatorAccepted || s.MessageID != "message-1" || s.AcceptedAt.IsZero() || s.OwnerToken != f.lease.record.OwnerToken || s.ClaimToken == s.OwnerToken || !s.ExpiresAt.Equal(expiry) {
			t.Fatal("publication did not preserve confirmed acceptance")
		}
		a, _ := got.Record().Encode()
		b, _ := f.saved(t).Encode()
		if !bytes.Equal(a, b) || f.reads.Load() != 2 || f.sends.Load() != 1 || len(f.objects.noticeWrites()) != 0 || preparationCalls(f.objects, "LIST", "") != 0 {
			t.Fatal("incorrect saved bytes or effects")
		}
		clear(s.Payload)
		if !bytes.Equal(got.Record().Snapshot().Payload, f.saved(t).Snapshot().Payload) {
			t.Fatal("caller mutated retained payload")
		}
		for range 2 {
			again, e := f.coordinator.Run(leaseBudget(t), f.lease)
			body, _ := again.Record().Encode()
			if e != nil || again.Dispatched() || again.ObservedStatus() != 0 || again.ObservedMessageID() != "" || !bytes.Equal(a, body) || f.reads.Load() != 2 || f.sends.Load() != 1 {
				t.Fatal("repeat resent or changed saved acceptance", e)
			}
		}
	}

}

func TestOperatorCoordinatorConservativelyClassifiesPublication(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		id       string
		err      error
		accepted bool
	}{
		{"accepted", 200, "message", nil, true}, {"unknown", 0, "message", errors.New("private"), false},
		{"response-with-error", 200, "message", errors.New("private"), false}, {"no-id", 200, "", nil, false},
		{"invalid-id", 200, "private value", nil, false}, {"long-id", 200, strings.Repeat("m", 257), nil, false},
		{"accepted202", 202, "message", nil, true},
		{"202-with-error", 202, "message", errors.New("private"), false}, {"202-no-id", 202, "", nil, false},
		{"202-invalid-id", 202, "bad id", nil, false}, {"other-status-id", 201, "message", nil, false}, {"retryable", 429, "", nil, false}, {"server", 503, "", nil, false},
		{"invalid-status", 600, "message", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			f.coordinator.read = func(context.Context) (operatorObservation, error) {
				f.reads.Add(1)
				return operatorObservation{f.coordinator.desired, func(context.Context, []byte) (ocinotifications.Publication, error) {
					f.sends.Add(1)
					return ocinotifications.Publication{Status: tc.status, MessageID: tc.id}, tc.err
				}}, nil
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			status := tc.status
			if status < 100 || status > 599 {
				status = 0
			}
			state, id := OperatorReview, ""
			if tc.accepted {
				state, id = OperatorAccepted, tc.id
			}
			s := got.Record().Snapshot()
			if !got.Dispatched() || got.ObservedStatus() != status || got.ObservedMessageID() != id || s.State != state || s.HTTPStatus != status || s.MessageID != id {
				t.Fatal("incorrect observed/saved outcome")
			}
			again, e := f.coordinator.Run(leaseBudget(t), f.lease)
			if e != nil || again.Dispatched() || f.sends.Load() != 1 || f.reads.Load() != 2 {
				t.Fatal("terminal request retried", e)
			}
		})
	}
}
