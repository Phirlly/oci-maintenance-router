package reconciliation

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

var _ BaselineObjects = (*ocistorage.Client)(nil)

func TestBaselineStoreCreatesExactBytesAndReloadsAfterRestart(t *testing.T) {
	b := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	store, err := NewBaselineStore(objects, b.EnvironmentID())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(leaseBudget(t), b)
	if err != nil {
		t.Fatal("valid immutable baseline was not stored", err)
	}
	want, _ := b.Encode()
	got, _ := created.Encode()
	if !bytes.Equal(got, want) || len(objects.calls) != 1 || objects.calls[0].method != "CREATE" ||
		objects.calls[0].name != "baselines/initial.json" || !bytes.Equal(objects.calls[0].body, want) {
		t.Fatal("creation changed bytes, path or performed extra I/O")
	}
	restarted, err := NewBaselineStore(objects, b.EnvironmentID())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(leaseBudget(t), b.ID())
	if err != nil {
		t.Fatal("saved baseline not readable after restart", err)
	}
	got, _ = loaded.Encode()
	if !bytes.Equal(got, want) || len(objects.calls) != 2 || objects.calls[1].method != "GET" || objects.calls[1].name != "baselines/initial.json" {
		t.Fatal("restart changed bytes, path or performed extra I/O")
	}
}
