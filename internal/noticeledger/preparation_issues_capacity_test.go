package noticeledger

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestPreparationIssuesBoundedHistoryNeverEvictsEarlierSubjects(t *testing.T) {
	ids := make([]string, 256)
	indexes := make([]int, 256)
	for i := range ids {
		ids[i] = fmt.Sprintf("ocid1.mysqldbsystem.oc1.iad.db%03d", i)
		indexes[i] = i
	}
	o := observer(t, nil)
	var r Record
	var p PreparationIssues
	for revision := 0; revision < 9; revision++ {
		raw := source(ids...)
		raw.Reference = text(fmt.Sprint(revision))
		n := normalized(t, raw)
		at := clock().Add(time.Duration(revision) * time.Second)
		var err error
		r, err = o.Observe(r, n, false, lookups(ids...), at)
		if err != nil {
			t.Fatal(err)
		}
		f, err := o.PreparationFailure(r, n, false, lookups(ids...), indexes, "state_capacity", at)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := p.Encode()
		next, err := p.Observe(f)
		if revision < 8 {
			if err != nil {
				t.Fatal("within-bound evidence rejected", revision, err)
			}
			p = next
			continue
		}
		if e, ok := err.(*Error); !ok || e.Code != "too_large" || next.Snapshot().SchemaVersion != 0 {
			t.Fatal("history bound not enforced", err)
		}
		after, _ := p.Encode()
		if !bytes.Equal(before, after) || len(p.Snapshot().Observations) != 2048 || len(after) > recordLimit {
			t.Fatal("capacity discarded previous evidence")
		}
	}
}
