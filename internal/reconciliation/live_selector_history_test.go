package reconciliation

import (
	"context"
	"testing"
)

func TestLiveSelectorRequiresPreparedHeadAndRetainedHistoryAdmission(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "corrupt", "retained"} {
		f := liveSelectorFixtureFor(t)
		code := "history_retained"
		switch kind {
		case "missing":
			delete(f.objects.objects, preparedHistoryName)
			code = "read_failed"
		case "empty":
			h, _ := NewPreparedHistory(f.lease.manager.environmentID, "installation-1")
			body, _ := h.Encode()
			f.objects.seed(preparedHistoryName, body)
			code = "baseline_unprepared"
		case "corrupt":
			f.objects.seed(preparedHistoryName, []byte("private"))
			code = "invalid_record"
		case "retained":
			f.objects.history = func(context.Context, string) (bool, error) { return false, nil }
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, code)
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
			t.Fatal("invalid history initialized selection")
		}
	}
}
