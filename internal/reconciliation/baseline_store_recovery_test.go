package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreConfirmsExactRepeatAndRejectsChangedIDContent(t *testing.T) {
	store, s, b := baselineStoreFixture(t)
	if _, err := store.Create(leaseBudget(t), b); err != nil {
		t.Fatal(err)
	}
	got, err := store.Create(leaseBudget(t), b)
	if err != nil || got.ID() != b.ID() {
		t.Fatal("exact repeat was not confirmed", err)
	}
	changed := buildBaseline(t, b.ID(), baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
	got, err = store.Create(leaseBudget(t), changed)
	baselineStoredFailure(t, got, err, "identity_conflict", 200, false)
	want, _ := b.Encode()
	if !bytes.Equal(s.objects["baselines/initial.json"].Body, want) {
		t.Fatal("reused ID overwrote original")
	}
	if len(s.calls) != 5 || s.calls[1].method != "CREATE" || s.calls[2].method != "GET" || s.calls[3].method != "CREATE" || s.calls[4].method != "GET" {
		t.Fatal("repeat did not use one conditional create and one confirmation")
	}
}

func TestBaselineStoreConfirmsLostWriteResponseWithoutRetransmission(t *testing.T) {
	for _, outcome := range []struct {
		name, tag string
		err       error
	}{
		{"transport", "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}},
		{"service", "", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}},
		{"malformed-response", "", &ocistorage.Error{Code: ocistorage.InvalidResponse, Status: 200, WriteUncertain: true}},
		{"missing-tag", "", nil}, {"invalid-tag", "*", nil},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			_, s, b := baselineStoreFixture(t)
			backend := &baselineBackend{get: s.Get, create: func(c context.Context, n string, body []byte) (string, error) {
				if _, err := s.Create(c, n, body); err != nil {
					t.Fatal(err)
				}
				// A backend retaining/mutating its upload must not change the
				// original baseline or the bytes used for confirmation.
				clear(body)
				return outcome.tag, outcome.err
			}}
			got, err := baselineStoreFor(t, backend, b.EnvironmentID()).Create(leaseBudget(t), b)
			want, _ := b.Encode()
			actual, _ := got.Encode()
			if err != nil || !bytes.Equal(actual, want) || len(s.calls) != 2 || s.calls[1].method != "GET" {
				t.Fatal("exact read-back did not confirm once", err)
			}
		})
	}
}

func TestBaselineStoreFailedConfirmationPreservesUncertainty(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	changed := buildBaseline(t, b.ID(), baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
	different, _ := changed.Encode()
	for _, tt := range []struct {
		name   string
		body   []byte
		err    error
		code   string
		status int
	}{
		{"missing", nil, &ocistorage.Error{Code: ocistorage.Service, Status: 404}, "read_failed", 404},
		{"invalid", []byte("private malformed"), nil, "invalid_record", 200},
		{"changed", different, nil, "identity_conflict", 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes, reads := 0, 0
			backend := &baselineBackend{
				create: func(context.Context, string, []byte) (string, error) {
					writes++
					return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				},
				get: func(context.Context, string) (ocistorage.Object, error) {
					reads++
					return ocistorage.Object{Body: tt.body, ETag: "v1"}, tt.err
				},
			}
			got, err := baselineStoreFor(t, backend, b.EnvironmentID()).Create(leaseBudget(t), b)
			baselineStoredFailure(t, got, err, tt.code, tt.status, true)
			if writes != 1 || reads != 1 {
				t.Fatal("confirmation failure retried")
			}
		})
	}
}
