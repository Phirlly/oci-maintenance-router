package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionStoreRejectsInvalidBindingBeforeIO(t *testing.T) {
	s, objects, backend, first, second := selectionStoreFixture(t)
	var nilBackend *selectionBackend
	for _, b := range []BaselineSelectionObjects{nil, nilBackend} {
		if got, err := NewBaselineSelectionStore(b, first.EnvironmentID(), "installation-1"); err == nil || got != nil {
			t.Fatal("nil backend admitted")
		}
	}
	for _, id := range [][2]string{{"", "id"}, {first.EnvironmentID(), ""}, {first.EnvironmentID(), "../id"}} {
		if got, err := NewBaselineSelectionStore(backend, id[0], id[1]); err == nil || got != nil {
			t.Fatal("invalid identity admitted")
		}
	}
	initial := selectionFor(t, first)
	foreign, _ := NewBaselineSelection(first.EnvironmentID(), "other", first)
	proposal := advanceSelection(t, initial, first, second)
	for _, store := range []*BaselineSelectionStore{nil, {}} {
		got, err := store.Load(leaseBudget(t))
		selectionStoredFailure(t, got, err, "invalid_request", 0, false)
		got, err = store.Create(leaseBudget(t), initial)
		selectionStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	for _, p := range []BaselineSelection{BaselineSelection{}, foreign, proposal} {
		got, err := s.Create(leaseBudget(t), p)
		selectionStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	different, err := NewBaselineSelectionStore(backend, strings.Repeat("f", 64), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := different.Create(leaseBudget(t), initial)
	selectionStoredFailure(t, got, err, "invalid_request", 0, false)
	if len(objects.calls) != 0 {
		t.Fatal("invalid input dispatched")
	}
}

func TestBaselineSelectionStoreRejectsInvalidReadAndOwnsBytes(t *testing.T) {
	s, objects, backend, first, _ := selectionStoreFixture(t)
	selection := selectionFor(t, first)
	body, _ := selection.Encode()
	for _, tc := range []struct {
		tag  string
		body []byte
		code string
	}{
		{"", body, "invalid_response"}, {"W/tag", body, "invalid_response"}, {"*", body, "invalid_response"}, {"a,b", body, "invalid_response"}, {"a" + string([]byte{13, 10}) + "b", body, "invalid_response"},
		{"tag", []byte("private"), "invalid_record"}, {"tag", make([]byte, 4097), "invalid_record"}, {"tag", []byte(strings.Replace(string(body), "installation-1", "other", 1)), "invalid_record"},
	} {
		calls := 0
		backend.get = func(context.Context, string) (ocistorage.Object, error) {
			calls++
			return ocistorage.Object{Body: tc.body, ETag: tc.tag}, nil
		}
		for _, call := range []func() (StoredBaselineSelection, error){func() (StoredBaselineSelection, error) { return s.Load(leaseBudget(t)) }, func() (StoredBaselineSelection, error) { return s.Create(leaseBudget(t), selection) }} {
			got, err := call()
			selectionStoredFailure(t, got, err, tc.code, 200, false)
		}
		if calls != 2 || objects.version != 0 {
			t.Fatal("invalid read retried or created state")
		}
	}
	backend.get = func(context.Context, string) (ocistorage.Object, error) {
		return ocistorage.Object{Body: body, ETag: `"valid-tag"`}, nil
	}
	got, err := s.Load(leaseBudget(t))
	clear(body)
	if err != nil || !got.Selection().Matches(first) {
		t.Fatal("stored state retained mutable response bytes", err)
	}
}
