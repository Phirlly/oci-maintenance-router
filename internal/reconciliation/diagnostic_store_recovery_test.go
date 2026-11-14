package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
)

func TestDiagnosticStoreConfirmsLostWritesOnce(t *testing.T) {
	for _, save := range []bool{false, true} {
		for _, outcome := range []struct {
			name, tag string
			err       error
		}{
			{"lost response", "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}},
			{"uncertain service", "", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}},
			{"missing etag", "", nil}, {"invalid etag", "*", nil},
		} {
			t.Run(outcome.name, func(t *testing.T) {
				s, objects, b := diagnosticStoreFixture(t)
				initial := diagnosticFixture(t)
				var old StoredDiagnostic
				next := initial
				if save {
					old = diagnosticSaved(t, s)
					next = diagnosticOutcome(t, old, 202)
				}
				b.create = func(c context.Context, n string, body []byte) (string, error) {
					if _, err := objects.Create(c, n, body); err != nil {
						return "", err
					}
					clear(body)
					return outcome.tag, outcome.err
				}
				b.replace = func(c context.Context, n, tag string, body []byte) (string, error) {
					if _, err := objects.Replace(c, n, tag, body); err != nil {
						return "", err
					}
					clear(body)
					return outcome.tag, outcome.err
				}
				var got StoredDiagnostic
				var err error
				before := len(objects.calls)
				if save {
					got, err = s.Save(leaseBudget(t), old, next)
				} else {
					got, err = s.Create(leaseBudget(t), next)
				}
				actual, _ := got.Record().Encode()
				expected, _ := next.Encode()
				wantCalls := 4
				if save {
					wantCalls = 2
				}
				if err != nil || !bytes.Equal(actual, expected) || len(objects.calls) != before+wantCalls || objects.calls[len(objects.calls)-1].method != "GET" {
					t.Fatal("lost write not confirmed once with original bytes", err)
				}
				if !save {
					if _, err = s.Save(leaseBudget(t), got, diagnosticOutcome(t, got, 202)); err != nil {
						t.Fatal("confirmed handle unusable", err)
					}
				}
			})
		}
	}
}

func TestDiagnosticFailedConfirmationPreservesUncertainty(t *testing.T) {
	for _, save := range []bool{false, true} {
		for _, kind := range []string{"missing", "corrupt", "different", "invalid-tag"} {
			t.Run(kind, func(t *testing.T) {
				s, objects, b := diagnosticStoreFixture(t)
				var old StoredDiagnostic
				next := diagnosticFixture(t)
				if save {
					old = diagnosticSaved(t, s)
					next = diagnosticOutcome(t, old, 202)
				}
				written := false
				b.create = func(context.Context, string, []byte) (string, error) {
					written = true
					return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				b.replace = func(context.Context, string, string, []byte) (string, error) {
					written = true
					return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				reads := 0
				b.get = func(c context.Context, n string) (ocistorage.Object, error) {
					if !written {
						return objects.Get(c, n)
					}
					reads++
					if kind == "missing" {
						return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
					}
					if kind == "corrupt" {
						return ocistorage.Object{Body: []byte("private"), ETag: "v9"}, nil
					}
					r := diagnosticFixture(t)
					if !save {
						i := diagnosticIntentFixture()
						i.PagerDutyRegion = "EU"
						r, _ = NewDiagnostic(i, 7, diagnosticClaimFixture())
					}
					body, _ := r.Encode()
					tag := "v9"
					if kind == "invalid-tag" {
						tag = "*"
					}
					return ocistorage.Object{Body: body, ETag: tag}, nil
				}
				var got StoredDiagnostic
				var err error
				if save {
					got, err = s.Save(leaseBudget(t), old, next)
				} else {
					got, err = s.Create(leaseBudget(t), next)
				}
				code, status := "identity_conflict", 200
				switch kind {
				case "missing":
					code, status = "read_failed", 404
				case "corrupt":
					code = "invalid_record"
				case "invalid-tag":
					code = "invalid_response"
				}
				diagnosticStoredFailure(t, got, err, code, status, true)
				if reads != 1 {
					t.Fatal("failed confirmation retried")
				}
			})
		}
	}
}

func TestDiagnosticCreateConflictPreservesProgressedOtherClaim(t *testing.T) {
	s, objects, b := diagnosticStoreFixture(t)
	initial := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	winner, err := NewDiagnostic(diagnosticIntentFixture(), 9, c)
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.Finish(diagnosticReference(c), 202, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	b.create = func(ctx context.Context, n string, body []byte) (string, error) {
		saved, _ := winner.Encode()
		objects.seed(n, saved)
		return objects.Create(ctx, n, body)
	}
	got, err := s.Create(leaseBudget(t), initial)
	if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || got.Record().Snapshot().SecretVersion != 9 || got.Record().Snapshot().ClaimToken != c.ClaimToken || objects.version != 1 {
		t.Fatal("conflict did not preserve first completed claim", err)
	}
}
