package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
)

func TestDiagnosticStoreRejectsInvalidConstructionAndRequests(t *testing.T) {
	s, objects, b := diagnosticStoreFixture(t)
	i := diagnosticIntentFixture()
	for _, tc := range []struct {
		objects      DiagnosticObjects
		env, install string
	}{{nil, i.EnvironmentID, i.InstallationID}, {(*diagnosticBackend)(nil), i.EnvironmentID, i.InstallationID}, {b, "wrong", i.InstallationID}, {b, i.EnvironmentID, "../other"}, {b, i.EnvironmentID, strings.Repeat("a", 129)}} {
		got, err := NewDiagnosticStore(tc.objects, tc.env, tc.install)
		if err == nil || got != nil {
			t.Fatal("invalid store constructed")
		}
	}
	before := len(objects.calls)
	for _, store := range []*DiagnosticStore{nil, {}, s} {
		for _, request := range []string{"", "../other", strings.Repeat("a", 129)} {
			got, err := store.Load(leaseBudget(t), request)
			diagnosticStoredFailure(t, got, err, "invalid_request", 0, false)
		}
		if store != s {
			got, err := store.Create(leaseBudget(t), diagnosticFixture(t))
			diagnosticStoredFailure(t, got, err, "invalid_request", 0, false)
		}
	}
	for _, field := range []string{"environment", "installation"} {
		other := i
		if field == "environment" {
			other.EnvironmentID = strings.Repeat("f", 64)
		} else {
			other.InstallationID = "other"
		}
		r, err := NewDiagnostic(other, 7, diagnosticClaimFixture())
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Create(leaseBudget(t), r)
		diagnosticStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before {
		t.Fatal("invalid request reached backend")
	}
}

func TestDiagnosticStoreRejectsMalformedForeignAndOversizedReads(t *testing.T) {
	for _, kind := range []string{"corrupt", "oversize", "foreign", "bad-tag"} {
		t.Run(kind, func(t *testing.T) {
			s, objects, b := diagnosticStoreFixture(t)
			r := diagnosticFixture(t)
			if kind == "foreign" {
				i := diagnosticIntentFixture()
				i.RequestID = "other"
				r, _ = NewDiagnostic(i, 7, diagnosticClaimFixture())
			}
			body, _ := r.Encode()
			tag := "v1"
			code := "invalid_record"
			switch kind {
			case "corrupt":
				body = []byte("private")
			case "oversize":
				body = make([]byte, diagnosticLimit+1)
			case "bad-tag":
				tag = "W/weak"
				code = "invalid_response"
			}
			b.get = func(context.Context, string) (ocistorage.Object, error) {
				return ocistorage.Object{Body: body, ETag: tag}, nil
			}
			b.history = func(context.Context, string) (bool, error) { t.Fatal("bad state treated as missing"); return true, nil }
			got, err := s.Load(leaseBudget(t), "test-1")
			diagnosticStoredFailure(t, got, err, code, 200, false)
			got, err = s.Create(leaseBudget(t), diagnosticFixture(t))
			diagnosticStoredFailure(t, got, err, code, 200, false)
			if objects.version != 0 {
				t.Fatal("invalid current state overwritten")
			}
		})
	}
}

func TestDiagnosticMaximumKeyFitsBackendAndRemainsScoped(t *testing.T) {
	_, _, b := diagnosticStoreFixture(t)
	i := diagnosticIntentFixture()
	i.InstallationID = strings.Repeat("I", 128)
	i.RequestID = strings.Repeat("R", 128)
	s, err := NewDiagnosticStore(b, i.EnvironmentID, i.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewDiagnostic(i, 7, diagnosticClaimFixture())
	if err != nil {
		t.Fatal(err)
	}
	var key string
	b.history = func(_ context.Context, n string) (bool, error) { key = n; return true, nil }
	got, err := s.Create(leaseBudget(t), r)
	if err != nil || len(key) > 1024 || key != "control/diagnostics/"+i.InstallationID+"/"+i.RequestID+".json" || got.Record().Snapshot().Intent != i {
		t.Fatal("maximum valid identifiers failed", err)
	}
}
