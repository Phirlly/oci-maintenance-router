package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestDiscoveryConfirmsEmptyCollection(t *testing.T) {
	d, s, lease := discoveryFixture(t, nil, nil)
	result, err := d.Run(leaseBudget(t), lease)
	if err != nil || result.EnvironmentID() != lease.manager.environmentID || result.Candidates().EnvironmentID() != result.EnvironmentID() || result.Availability().EnvironmentID() != result.EnvironmentID() || result.CompletedAt().IsZero() {
		t.Fatal("empty complete discovery failed", err)
	}
	if len(s.noticeWrites()) != 0 || len(result.Candidates().Components()) != 0 {
		t.Fatal("empty discovery invented work")
	}
}

func TestDiscoveryPersistsOriginalOccurrencesAndAllChainHolds(t *testing.T) {
	first := candidateNotice(t, 1, chain("shared"), false)
	linked := candidateNotice(t, 2, chain("shared"), true)
	changedSource := first.Snapshot()
	changedSource.Summary = chain("changed source")
	changed := normalizeCandidate(t, changedSource)
	d, s, lease := discoveryFixture(t, []notices.Notice{first, linked, first, changed}, nil)
	result, err := d.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	components := result.Candidates().Components()
	if len(components) != 1 || len(components[0].Members) != 3 {
		t.Fatal("lost related revision")
	}
	for _, id := range []string{first.Key().NoticeID, linked.Key().NoticeID} {
		r := s.record(t, result.EnvironmentID(), id).Snapshot()
		if r.HoldReason != delivery.ReasonSourceChanged {
			t.Fatal("component member not held")
		}
	}
	var occurrences []string
	for _, call := range s.noticeWrites() {
		if call.name != noticeName(first.Key().NoticeID) {
			continue
		}
		r, err := noticeledger.Decode(call.body, result.EnvironmentID(), first.Key().NoticeID)
		if err != nil {
			t.Fatal(err)
		}
		if outcome := r.Snapshot().SourceOutcome; outcome != nil && outcome.State == noticeledger.SourceAvailable {
			occurrences = append(occurrences, outcome.RevisionDigest)
		}
	}
	if len(occurrences) < 3 || occurrences[0] != first.Key().Digest || occurrences[1] != first.Key().Digest || occurrences[2] != changed.Key().Digest {
		t.Fatal("original per-notice occurrence order lost", occurrences)
	}
}
