package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparedHistoryStoreConfirmsLostWriteOnceWithOwnedBytes(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, outcome := range []struct {
			tag string
			err error
		}{
			{"", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}},
			{"", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}},
			{"", nil}, {"*", nil},
		} {
			_, objects, first, _ := preparedStoreFixture(t)
			lose := false
			backend := &preparedBackend{get: objects.Get,
				create: func(c context.Context, n string, b []byte) (string, error) {
					tag, err := objects.Create(c, n, b)
					if err != nil || !lose {
						return tag, err
					}
					clear(b)
					return outcome.tag, outcome.err
				},
				replace: func(c context.Context, n, tag string, b []byte) (string, error) {
					saved, err := objects.Replace(c, n, tag, b)
					if err != nil {
						return saved, err
					}
					clear(b)
					return outcome.tag, outcome.err
				},
			}
			s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
			var old StoredPreparedHistory
			var next PreparedHistory
			if !create {
				var err error
				old, err = s.Create(leaseBudget(t))
				if err != nil {
					t.Fatal(err)
				}
				next = advancePrepared(t, old.History(), Baseline{}, first)
			}
			before := len(objects.calls)
			lose = true
			var got StoredPreparedHistory
			var err error
			if create {
				got, err = s.Create(leaseBudget(t))
			} else {
				got, err = s.Save(leaseBudget(t), old, next)
			}
			if err != nil || got.History().EnvironmentID() != first.EnvironmentID() || (!create && got.History().BaselineDigest() != next.BaselineDigest()) || len(objects.calls) != before+2 || objects.calls[before+1].method != "GET" {
				t.Fatal("lost response not confirmed once", err)
			}
			if create {
				next = advancePrepared(t, got.History(), Baseline{}, first)
				if _, err := s.Save(leaseBudget(t), got, next); err != nil {
					t.Fatal("confirmed tag unusable", err)
				}
			}
		}
	}
}

func TestPreparedHistoryStoreFailedConfirmationPreservesUncertainty(t *testing.T) {
	for _, outcome := range []string{"different", "missing", "malformed"} {
		_, objects, first, _ := preparedStoreFixture(t)
		backend := &preparedBackend{get: objects.Get, create: objects.Create, replace: func(context.Context, string, string, []byte) (string, error) {
			return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}}
		s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
		old, err := s.Create(leaseBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		next := advancePrepared(t, old.History(), Baseline{}, first)
		reads := 0
		backend.get = func(c context.Context, n string) (ocistorage.Object, error) {
			reads++
			if outcome == "missing" {
				return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
			}
			if outcome == "malformed" {
				return ocistorage.Object{Body: []byte("private data"), ETag: "v2"}, nil
			}
			return objects.Get(c, n)
		}
		got, err := s.Save(leaseBudget(t), old, next)
		code, status := "identity_conflict", 200
		if outcome == "missing" {
			code, status = "read_failed", 404
		}
		if outcome == "malformed" {
			code = "invalid_record"
		}
		preparedStoredFailure(t, got, err, code, status, true)
		if reads != 1 || objects.version != 1 {
			t.Fatal("failed confirmation changed history or retried")
		}
	}
}
