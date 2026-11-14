package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLiveSelectorRejectsFuturePreparedCompletion(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	cfg := preparationConfig(t, func(v map[string]any) { v["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment} })
	p := preparationFor(t, f.objects, cfg, nil)
	p.discovery.now = func() time.Time { return f.lease.manager.now().Add(time.Hour) }
	prepareSuccessfully(t, p, f.lease)
	f.configure(t)
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	liveSelectionFailure(t, b, err, "invalid_time")
	if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
		t.Fatal("future baseline selected")
	}
}

func TestLiveSelectorRejectedStorageNeverAdvances(t *testing.T) {
	for _, stage := range []string{"target", "history", "create", "replace"} {
		f := liveSelectorFixtureFor(t)
		if stage == "replace" {
			f.selectOK(t)
			f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
			f.configure(t)
		}
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			match := stage == "target" && c.method == "GET" && c.name == "baselines/"+f.baseline.ID()+".json" || c.name == baselineSelectionName && (stage == "history" && c.method == "HISTORY" || stage == "create" && c.method == "CREATE" || stage == "replace" && c.method == "REPLACE")
			if match {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		code := "write_failed"
		if stage == "target" {
			code = "read_failed"
		}
		if stage == "history" {
			code = "history_read"
		}
		e := liveSelectionFailure(t, b, err, code)
		if e.HTTPStatus != 403 || e.WriteUncertain {
			t.Fatal("definite rejection misclassified")
		}
	}
}

func TestLiveSelectorChecksAuthorityAfterPredecessorRead(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.selectOK(t)
	f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
	f.configure(t)
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "GET" && c.name == "baselines/"+f.baseline.ID()+".json" {
			f.objects.seed("configuration/router.json", []byte("new"))
		}
		return nil
	}
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	liveSelectionFailure(t, b, err, "configuration_changed")
	if preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 {
		t.Fatal("stale input advanced selection")
	}
}

func TestLiveSelectorRepeatDoesNotTrustDeletedAcceptedState(t *testing.T) {
	for _, name := range []string{diagnosticObjectName, operatorRunObjectName} {
		f := liveSelectorFixtureFor(t)
		f.selectOK(t)
		delete(f.objects.objects, name)
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, "read_failed")
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 {
			t.Fatal("missing proof changed selection")
		}
	}
}
