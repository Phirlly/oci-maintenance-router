package reconciliation

import (
	"bytes"
	"testing"
)

func TestBaselineSelectionStoreConditionalLifecycle(t *testing.T) {
	s, objects, backend, first, second := selectionStoreFixture(t)
	ctx := leaseBudget(t)
	missing, err := s.Load(ctx)
	selectionStoredFailure(t, missing, err, "read_failed", 404, false)
	if len(objects.calls) != 1 || objects.version != 0 {
		t.Fatal("missing load initialized state")
	}
	initial := selectionFor(t, first)
	created, err := s.Create(ctx, initial)
	if err != nil || !created.Selection().Matches(first) {
		t.Fatal("first nonempty selection failed", err)
	}
	if len(objects.calls) != 4 || objects.calls[1].method != "GET" || objects.calls[2].method != "HISTORY" || objects.calls[3].method != "CREATE" {
		t.Fatal("creation omitted absence/history guard")
	}
	repeated, err := s.Create(ctx, initial)
	if err != nil || !repeated.Selection().Matches(first) || objects.version != 1 || len(objects.calls) != 5 {
		t.Fatal("exact repeat wrote or changed selection", err)
	}
	loaded, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := advanceSelection(t, loaded.Selection(), first, second)
	saved, err := s.Save(ctx, loaded, next)
	if err != nil || !saved.Selection().Matches(second) || objects.version != 2 || objects.calls[6].tag != "v1" {
		t.Fatal("conditional advance failed", err)
	}
	for _, c := range objects.calls {
		if c.name != selectionObjectName {
			t.Fatal("wrong selection key")
		}
	}
	want, _ := next.Encode()
	if !bytes.Equal(objects.objects[selectionObjectName].Body, want) || !created.Selection().Matches(first) {
		t.Fatal("stored bytes or old handle changed")
	}
	got, err := s.Create(ctx, initial)
	selectionStoredFailure(t, got, err, "identity_conflict", 200, false)
	if objects.version != 2 {
		t.Fatal("old create rewound selection")
	}
	// A new process reads the actual cumulative state without creating it.
	restarted := selectionStoreFor(t, backend, first)
	got, err = restarted.Load(ctx)
	if err != nil || !got.Selection().Matches(second) {
		t.Fatal("restart lost selection", err)
	}
}
