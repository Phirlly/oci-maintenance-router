package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestCollectionStartsFreshAfterFailureAndOwnsResults(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	a, s := detailSources(t, []notices.Notice{n}, []noticeledger.Record{r})
	reads := 0
	a.get = func(context.Context, string) (notices.Notice, error) {
		reads++
		if reads == 1 {
			return notices.Notice{}, errors.New("unavailable")
		}
		return n, nil
	}
	s.load = func(context.Context, string) (noticeledger.Record, error) { return r, nil }
	data, catalog := []byte(candidateConfig), candidateCatalog()
	c, err := NewCollector(data, catalog, candidateLocation(), a, s)
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	clear(catalog.Regions)
	catalog.MySQLServices[0] = "changed"
	got, err := c.collect(budget(t), noPause)
	wantCollectionFailure(t, got, err, ReadFailed, Announcements, 0)
	prior, err := c.collect(budget(t), noPause)
	if err != nil || len(prior.Current) != 1 || len(prior.Saved) != 1 || !prior.Current[0].KnownCandidate {
		t.Fatalf("failed recovery: %+v %v", prior, err)
	}
	prior.Current[0].Revision.NoticeID = "caller edit"
	prior.Current[0].KnownCandidate = false
	prior.Saved[0] = SavedNoticeRef{ObjectName: "caller edit", NoticeID: "caller edit"}
	got, err = c.collect(budget(t), noPause)
	if err != nil || len(got.Current) != 1 || len(got.Saved) != 1 || got.Current[0].Revision != n.Key() || !got.Current[0].KnownCandidate || got.Saved[0].ObjectName != recordName(r) || got.Saved[0].NoticeID != n.Key().NoticeID {
		t.Fatalf("result isolation failed: %+v %v", got, err)
	}
}

func TestCollectorConcurrentInvocationsHaveIndependentState(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	r := candidateRecord(t, n)
	a, s := detailSources(t, []notices.Notice{n}, []noticeledger.Record{r})
	a.get = func(context.Context, string) (notices.Notice, error) { return n, nil }
	s.load = func(context.Context, string) (noticeledger.Record, error) { return r, nil }
	c := collector(t, a, s)
	for i := range 8 {
		t.Run(noticeID(i), func(t *testing.T) {
			t.Parallel()
			got, err := c.collect(budget(t), noPause)
			if err != nil || len(got.Current) != 1 || len(got.Saved) != 1 || !got.Current[0].KnownCandidate {
				t.Fatalf("shared collection state: %+v %v", got, err)
			}
		})
	}
}
