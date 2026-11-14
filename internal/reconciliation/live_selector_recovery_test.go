package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLiveSelectorConfirmsLostFirstAndAdvanceWritesWithoutRetry(t *testing.T) {
	for _, advance := range []bool{false, true} {
		f := liveSelectorFixtureFor(t)
		want := f.baseline
		if advance {
			f.selectOK(t)
			want = f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
			f.configure(t)
		}
		f.objects.after = func(_ context.Context, c discoveryCall) error {
			if c.name == baselineSelectionName && (c.method == "CREATE" || c.method == "REPLACE") {
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		b := f.selectOK(t)
		if b.ID() != want.ID() {
			t.Fatal("wrong confirmed baseline")
		}
		f.configure(t)
		f.selectOK(t)
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != map[bool]int{false: 0, true: 1}[advance] {
			t.Fatal("lost response retried mutation")
		}
	}
}

func TestLiveSelectorDifferentConfirmedWinnerNeverBecomesSuccess(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.selectOK(t)
	old := f.objects.objects[baselineSelectionName]
	f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
	f.configure(t)
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == baselineSelectionName && c.method == "REPLACE" {
			f.objects.seed(baselineSelectionName, old.Body)
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	e := liveSelectionFailure(t, b, err, "identity_conflict")
	if !e.WriteUncertain || preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 1 {
		t.Fatal("different winner rebased or lost uncertainty")
	}
}

func TestLiveSelectorRawErrorsCannotForgeMissingOrExposeDetails(t *testing.T) {
	for _, failure := range []error{errors.New("private"), &BaselineStorageError{Code: "read_failed", HTTPStatus: 404}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404})} {
		f := liveSelectorFixtureFor(t)
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if c.name == baselineSelectionName {
				return failure
			}
			return nil
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		e := liveSelectionFailure(t, b, err, "read_failed")
		if e.HTTPStatus != 0 || err.Error() != "live selection: read_failed" || preparationCalls(f.objects, "HISTORY", baselineSelectionName) != 0 {
			t.Fatal("raw error supplied missing provenance or private text")
		}
	}
}
