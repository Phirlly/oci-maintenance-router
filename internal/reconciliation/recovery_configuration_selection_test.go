package reconciliation

import (
	"bytes"
	"testing"
)

func TestRecoveryConfigurationPreservesLiveSelectionAndReadiness(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	want, _ := f.selectOK(t).Encode()
	old := f.selector
	for i, id := range []string{"recovery-1", "recovery-2"} {
		f.configure(t, func(v map[string]any) { recoveryConfiguration(v); v["recovery"].(map[string]any)["requestId"] = id })
		if i == 0 {
			got, err := old.Select(leaseBudget(t), f.lease)
			liveSelectionFailure(t, got, err, "configuration_changed")
		}
		got, _ := f.selectOK(t).Encode()
		if !bytes.Equal(want, got) {
			t.Fatal("recovery changed selected baseline")
		}
	}
	if f.channelReads.Load() != 3 || preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 {
		t.Fatal("recovery bypassed readiness or rewrote selection")
	}
	f.configure(t, func(v map[string]any) {
		recoveryConfiguration(v)
		v["diagnostic"].(map[string]any)["requestId"] = "unaccepted"
	})
	got, err := f.selector.Select(leaseBudget(t), f.lease)
	liveSelectionFailure(t, got, err, "read_failed")
}
