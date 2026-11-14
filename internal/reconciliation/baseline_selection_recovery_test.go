package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionStoreConfirmsLostWriteExactlyOnce(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, outcome := range []struct {
			tag string
			err error
		}{
			{"", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}},
			{"", &ocistorage.Error{Code: ocistorage.Service, Status: 503, WriteUncertain: true}},
			{"", nil}, {"*", nil},
		} {
			s, objects, backend, first, second := selectionStoreFixture(t)
			next := selectionFor(t, first)
			var old StoredBaselineSelection
			if !create {
				old = selectionSaved(t, s, first)
				next = advanceSelection(t, old.Selection(), first, second)
			}
			backend.create = func(c context.Context, n string, b []byte) (string, error) {
				if _, err := objects.Create(c, n, b); err != nil {
					t.Fatal(err)
				}
				clear(b)
				return outcome.tag, outcome.err
			}
			backend.replace = func(c context.Context, n, tag string, b []byte) (string, error) {
				if _, err := objects.Replace(c, n, tag, b); err != nil {
					t.Fatal(err)
				}
				clear(b)
				return outcome.tag, outcome.err
			}
			before := len(objects.calls)
			var got StoredBaselineSelection
			var err error
			if create {
				got, err = s.Create(leaseBudget(t), next)
			} else {
				got, err = s.Save(leaseBudget(t), old, next)
			}
			calls := 2
			if create {
				calls = 4
			}
			if err != nil || got.Selection().BaselineDigest() != next.BaselineDigest() || len(objects.calls) != before+calls || objects.calls[len(objects.calls)-1].method != "GET" {
				t.Fatal("lost write not exactly confirmed", err)
			}
			if create {
				p := advanceSelection(t, got.Selection(), first, second)
				if _, err = s.Save(leaseBudget(t), got, p); err != nil {
					t.Fatal("confirmed handle unusable", err)
				}
			}
		}
	}
}

func TestBaselineSelectionFailedConfirmationCannotAcceptAnotherWinner(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, kind := range []string{"different", "missing", "malformed", "tag"} {
			s, objects, backend, first, second := selectionStoreFixture(t)
			next := selectionFor(t, first)
			var old StoredBaselineSelection
			if !create {
				old = selectionSaved(t, s, first)
				next = advanceSelection(t, old.Selection(), first, second)
			}
			written := false
			fail := func() {
				written = true
				if kind == "different" {
					other := selectionFor(t, second)
					if !create {
						other = selectionFor(t, first)
					}
					body, _ := other.Encode()
					objects.seed(selectionObjectName, body)
				}
			}
			backend.create = func(context.Context, string, []byte) (string, error) {
				fail()
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			backend.replace = func(context.Context, string, string, []byte) (string, error) {
				fail()
				return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			reads := 0
			backend.get = func(c context.Context, n string) (ocistorage.Object, error) {
				if !written {
					return objects.Get(c, n)
				}
				reads++
				switch kind {
				case "missing":
					return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
				case "malformed":
					return ocistorage.Object{Body: []byte("private"), ETag: "v2"}, nil
				case "tag":
					return ocistorage.Object{ETag: "*"}, nil
				}
				return objects.Get(c, n)
			}
			var got StoredBaselineSelection
			var err error
			if create {
				got, err = s.Create(leaseBudget(t), next)
			} else {
				got, err = s.Save(leaseBudget(t), old, next)
			}
			code, status := "identity_conflict", 200
			switch kind {
			case "missing":
				code, status = "read_failed", 404
			case "malformed":
				code = "invalid_record"
			case "tag":
				code = "invalid_response"
			}
			selectionStoredFailure(t, got, err, code, status, true)
			if reads != 1 {
				t.Fatal("confirmation retried")
			}
		}
	}
}
