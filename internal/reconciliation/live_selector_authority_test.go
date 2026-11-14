package reconciliation

import (
	"context"
	"testing"
)

func TestLiveSelectorRejectsStaleControlsAndLostLeaseBeforeMutation(t *testing.T) {
	for _, stage := range []string{"config", "catalog", "lease", "corrupt-lease"} {
		f := liveSelectorFixtureFor(t)
		name := "configuration/router.json"
		code := "configuration_changed"
		if stage == "catalog" {
			name = "configuration/catalog.json"
		}
		if stage == "lease" || stage == "corrupt-lease" {
			name = workerLeaseName
			code = "ownership_lost"
		}
		if stage == "lease" {
			// Same valid bytes with a different saved version still lose ownership.
			f.objects.seed(name, f.objects.objects[name].Body)
		} else {
			f.objects.seed(name, []byte("changed"))
		}
		if stage == "corrupt-lease" {
			code = "invalid_record"
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, code)
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
			t.Fatal("stale authority wrote selection")
		}
	}
}

func TestLiveSelectorFinalGuardCoversWrittenAndNoOpPaths(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		f := liveSelectorFixtureFor(t)
		if repeat {
			f.selectOK(t)
		}
		changed := false
		f.objects.after = func(_ context.Context, c discoveryCall) error {
			if c.name == baselineSelectionName && (c.method == "CREATE" || repeat && c.method == "GET") && !changed {
				changed = true
				f.objects.seed("configuration/router.json", []byte("new"))
			}
			return nil
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, "configuration_changed")
		if !changed || preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 {
			t.Fatal("final-guard path not exercised")
		}
		f.objects.after = nil
		f.configure(t)
		f.selectOK(t)
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 {
			t.Fatal("retry reset confirmed selection")
		}
	}
}
