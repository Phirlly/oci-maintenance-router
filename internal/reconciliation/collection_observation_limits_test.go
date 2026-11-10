package reconciliation

import (
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func captureSize(t *testing.T, n notices.Notice) int {
	t.Helper()
	o, e := noticeledger.New([]byte(candidateConfig), candidateCatalog(), candidateLocation())
	if e != nil {
		t.Fatal(e)
	}
	c, e := o.Capture(n)
	if e != nil {
		t.Fatal(e)
	}
	return c.RetainedBytes()
}

func TestCollectionProjectionBudgetExactLimitAndOneByteOverflow(t *testing.T) {
	for _, seed := range []bool{true, false} {
		src := candidateNotice(t, 1, nil, seed).Snapshot()
		src.Reference = chain("")
		base := normalizeCandidate(t, src)
		room := (4 << 20) - 5*captureSize(t, base)
		var ns []notices.Notice
		for i := 0; i < 5; i++ {
			padding := room / 5
			if i == 4 {
				padding += room % 5
			}
			src.Reference = chain(strings.Repeat("x", padding))
			ns = append(ns, normalizeCandidate(t, src))
		}
		a, s := detailSources(t, ns, nil)
		got, e := collector(t, a, s).collect(budget(t), noPause)
		if e != nil || len(got.Current) != 5 || (seed && len(got.Observations) != 5) || (!seed && len(got.Observations) != 0) {
			t.Fatal("exact projection limit rejected", e)
		}
		last := ns[4].Snapshot()
		last.Reference = chain(*last.Reference + "x")
		ns[4] = normalizeCandidate(t, last)
		a, s = detailSources(t, ns, nil)
		got, e = collector(t, a, s).collect(budget(t), noPause)
		wantCollectionFailure(t, got, e, ByteLimit, Announcements, 0)
	}
}

func TestCollectionProjectionBudgetCountsEveryResourceAndDuplicate(t *testing.T) {
	src := candidateNotice(t, 1, nil, true).Snapshot()
	for i := 0; i < 256; i++ {
		src.Resources = append(src.Resources, notices.Resource{ResourceID: fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.%03d", i)})
	}
	n := normalizeCandidate(t, src)
	// More than 4MiB of complete rows, while ID/digest facts remain small.
	ns := make([]notices.Notice, 100)
	for i := range ns {
		ns[i] = n
	}
	a, s := detailSources(t, ns, nil)
	got, e := collector(t, a, s).collect(budget(t), noPause)
	wantCollectionFailure(t, got, e, ByteLimit, Announcements, 0)
}
