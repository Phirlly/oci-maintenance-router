package reconciliation

import (
	"sync"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestPreparationIssueStoreConcurrentCreatesDoNotRebase(t *testing.T) {
	s, _, _, f := preparationIssueFixture(t)
	a, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, previous := range []StoredPreparationIssues{a, b} {
		wg.Add(1)
		go func(previous StoredPreparationIssues) {
			defer wg.Done()
			next, err := previous.Record().Observe(f)
			if err == nil {
				_, err = s.Save(leaseBudget(t), previous, next)
			}
			results <- err
		}(previous)
	}
	wg.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else if e, ok := err.(*PreparationIssueStorageError); ok && e.Code == "conflict" && !e.WriteUncertain {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal("concurrent writes were both accepted", winners, conflicts)
	}
}

func TestPreparationIssueStoreRejectsForeignAndDecodedProposals(t *testing.T) {
	s, _, b, f := preparationIssueFixture(t)
	previous, err := s.Load(leaseBudget(t), f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	next, _ := previous.Record().Observe(f)
	other, err := NewPreparationIssueStore(b, s.environment)
	if err != nil {
		t.Fatal(err)
	}
	got, err := other.Save(leaseBudget(t), previous, next)
	issueStorageFailed(t, got, err, "invalid_request", 0, false)
	body, _ := next.Encode()
	decoded, err := noticeledger.DecodePreparationIssues(body, f.Notice())
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Save(leaseBudget(t), previous, decoded)
	issueStorageFailed(t, got, err, "invalid_request", 0, false)
	if _, err = s.Save(leaseBudget(t), previous, next); err != nil {
		t.Fatal("invalid proposal consumed absent handle", err)
	}
}
