package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
)

type preparationIssueBackend struct {
	*preparedBackend
	history func(context.Context, string) (bool, error)
}

func (b *preparationIssueBackend) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	return b.history(ctx, name)
}

func preparationIssueFixture(t *testing.T) (*PreparationIssueStore, *discoveryObjects, *preparationIssueBackend, noticeledger.PreparationFailure) {
	t.Helper()
	n := processingNotice(t, 1, processingDB)
	o, err := noticeledger.New([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: routing.Metadata{ID: chain(processingDB), CompartmentID: chain("ocid1.compartment.oc1..example"), FreeformTags: map[string]string{"owner": "payments"}}}}
	r, err := o.Observe(noticeledger.Record{}, n, true, lookups, leaseTime())
	if err != nil {
		t.Fatal(err)
	}
	f, err := o.PreparationFailure(r, n, true, lookups, []int{0}, "secret_unavailable", leaseTime())
	if err != nil {
		t.Fatal(err)
	}
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	b := &preparationIssueBackend{preparedBackend: &preparedBackend{get: objects.Get, create: objects.Create, replace: objects.Replace}, history: func(ctx context.Context, name string) (bool, error) {
		return true, objects.begin(ctx, discoveryCall{method: "HISTORY", name: name})
	}}
	env, _ := r.Identity()
	s, err := NewPreparationIssueStore(b, env)
	if err != nil {
		t.Fatal("valid issue store rejected", err)
	}
	return s, objects, b, f
}

func TestPreparationIssueStoreCreatesAndReplacesExactSavedParent(t *testing.T) {
	s, _, _, f := preparationIssueFixture(t)
	previous, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil || previous.Record().Snapshot().SchemaVersion != 0 {
		t.Fatal("first absence not established", err)
	}
	next, err := previous.Record().Observe(f)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(leaseBudget(t), previous, next)
	if err != nil {
		t.Fatal("first issue not saved", err)
	}
	loaded, err := s.Load(leaseBudget(t), f.Notice())
	x, _ := saved.Record().Encode()
	y, _ := loaded.Record().Encode()
	if err != nil || !bytes.Equal(x, y) {
		t.Fatal("restart lost issues", err)
	}
	proposal, err := loaded.Record().Observe(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(leaseBudget(t), loaded, proposal); err != nil {
		t.Fatal("exact child rejected", err)
	}
	if _, err = s.Save(leaseBudget(t), saved, proposal); err == nil {
		t.Fatal("old parent borrowed new proposal")
	}
	if _, err = s.Save(leaseBudget(t), previous, next); err == nil {
		t.Fatal("consumed absence overwrote state")
	}
}

func TestPreparationIssueStoreRetainedHistoryBlocksAbsentReset(t *testing.T) {
	s, _, b, f := preparationIssueFixture(t)
	b.history = func(context.Context, string) (bool, error) { return false, nil }
	got, err := s.Load(leaseBudget(t), f.Notice())
	if err == nil || got.store != nil {
		t.Fatal("retained deletion history reset", err)
	}
}

func TestPreparationIssueStorePreservesCallerLifetimeAndCanonicalPath(t *testing.T) {
	s, objects, b, f := preparationIssueFixture(t)
	_, id := f.Notice().Identity()
	want := fmt.Sprintf("review/preparation/%x.json", sha256.Sum256([]byte(id)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	var child context.Context
	b.get = func(c context.Context, name string) (ocistorage.Object, error) {
		child = c
		d, _ := c.Deadline()
		if d.After(deadline) || name != want {
			t.Error("wrong deadline/path")
		}
		return objects.Get(c, name)
	}
	if _, err := s.Load(ctx, f.Notice()); err != nil {
		t.Fatal(err)
	}
	if child == nil || child.Err() != context.Canceled {
		t.Fatal("storage child not cleaned up")
	}
}
