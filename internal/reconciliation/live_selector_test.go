package reconciliation

import (
	"bytes"
	"testing"
)

func TestLiveSelectorFirstRepeatAndRestartRequireActualReceipts(t *testing.T) {
	for _, status := range []int{200, 202} {
		f := liveSelectorStatusFixture(t, status)
		before, _ := f.baseline.Encode()
		for i := 0; i < 3; i++ {
			if i == 2 {
				f.configure(t)
			}
			b := f.selectOK(t)
			got, _ := b.Encode()
			if !bytes.Equal(before, got) {
				t.Fatal("selection changed prepared baseline")
			}
		}
		if f.channelReads.Load() != 3 || preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 || preparationCalls(f.objects, "HISTORY", baselineSelectionName) != 1 {
			t.Fatal("repeat bypassed readiness or rewrote selection")
		}
		obj, err := f.objects.Get(leaseBudget(t), baselineSelectionName)
		if err != nil {
			t.Fatal(err)
		}
		s, err := DecodeBaselineSelection(obj.Body, f.lease.manager.environmentID, "installation-1")
		if err != nil || !s.Matches(f.baseline) {
			t.Fatal("wrong persisted selection", err)
		}
	}

}

func TestLiveSelectorReceiptsMustBothRemainValidOnRepeat(t *testing.T) {
	for _, status := range []int{200, 202} {
		for _, field := range []string{"diagnostic", "operatorTest"} {
			f := liveSelectorStatusFixture(t, status)
			f.selectOK(t)
			f.configure(t, func(v map[string]any) { v[field].(map[string]any)["requestId"] = "other" })
			b, err := f.selector.Select(leaseBudget(t), f.lease)
			liveSelectionFailure(t, b, err, "read_failed")
			if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 {
				t.Fatal("invalid receipt changed selection")
			}
		}
	}

}
