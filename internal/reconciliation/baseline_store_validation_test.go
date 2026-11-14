package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreRejectsInvalidBindingsBeforeIO(t *testing.T) {
	_, s, b := baselineStoreFixture(t)
	var typedNil *baselineBackend
	for _, backend := range []BaselineObjects{nil, typedNil} {
		if store, err := NewBaselineStore(backend, b.EnvironmentID()); err == nil || store != nil {
			t.Fatal("nil backend accepted")
		}
	}
	for _, env := range []string{"", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if store, err := NewBaselineStore(s, env); err == nil || store != nil {
			t.Fatal("invalid environment accepted")
		}
	}
	valid := baselineStoreFor(t, s, b.EnvironmentID())
	foreign := baselineStoreFor(t, s, strings.Repeat("f", 64))
	for _, store := range []*BaselineStore{nil, {}, foreign} {
		got, err := store.Create(leaseBudget(t), b)
		baselineStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	for _, store := range []*BaselineStore{nil, {}} {
		got, err := store.Load(leaseBudget(t), b.ID())
		baselineStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	got, err := valid.Create(leaseBudget(t), Baseline{})
	baselineStoredFailure(t, got, err, "invalid_request", 0, false)
	for _, id := range []string{"", "../initial", "x/y", strings.Repeat("a", 129)} {
		got, err := valid.Load(leaseBudget(t), id)
		baselineStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(s.calls) != 0 {
		t.Fatal("invalid binding performed I/O")
	}
	zeroClient := baselineStoreFor(t, &ocistorage.Client{}, b.EnvironmentID())
	got, err = zeroClient.Create(leaseBudget(t), b)
	baselineStoredFailure(t, got, err, "write_failed", 0, false)
}

func TestBaselineStoreRejectsUntrustedReadRepresentations(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	body, _ := b.Encode()
	for _, tt := range []struct {
		name, tag string
		body      []byte
		code      string
	}{
		{"empty-tag", "", body, "invalid_response"},
		{"weak", "W/tag", body, "invalid_response"},
		{"wildcard", "*", body, "invalid_response"},
		{"list", "a,b", body, "invalid_response"},
		{"header", "a\r\nb", body, "invalid_response"},
		{"empty-quoted", `""`, body, "invalid_response"},
		{"unicode-tag", "é", body, "invalid_response"},
		{"long-tag", strings.Repeat("a", 1025), body, "invalid_response"},
		{"oversize", "v1", make([]byte, (1<<20)+1), "invalid_record"},
		{"malformed", "v1", []byte("private body"), "invalid_record"},
		{"noncanonical", "v1", append(append([]byte(nil), body...), ' '), "invalid_record"},
		{"foreign-environment", "v1", []byte(strings.ReplaceAll(string(body), b.EnvironmentID(), strings.Repeat("f", 64))), "invalid_record"},
		{"foreign-id", "v1", []byte(strings.ReplaceAll(string(body), `"initial"`, `"other"`)), "invalid_record"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			backend := &baselineBackend{get: func(_ context.Context, name string) (ocistorage.Object, error) {
				calls++
				if name != "baselines/initial.json" {
					t.Fatal("caller controlled path")
				}
				return ocistorage.Object{Body: tt.body, ETag: tt.tag}, nil
			}}
			got, err := baselineStoreFor(t, backend, b.EnvironmentID()).Load(leaseBudget(t), b.ID())
			baselineStoredFailure(t, got, err, tt.code, 200, false)
			if calls != 1 {
				t.Fatal("read retried")
			}
		})
	}
}
