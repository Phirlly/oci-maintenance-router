package reconciliation

import (
	"encoding/json"
	"testing"
)

func TestLiveSelectorRejectsMissingCorruptOrMismatchedStoredInputs(t *testing.T) {
	for _, kind := range []string{"target-missing", "target-digest", "selected-corrupt", "selected-foreign", "selected-digest", "previous-missing", "rewind"} {
		t.Run(kind, func(t *testing.T) {
			f := liveSelectorFixtureFor(t)
			code := "invalid_record"
			switch kind {
			case "target-missing":
				delete(f.objects.objects, "baselines/"+f.baseline.ID()+".json")
				code = "read_failed"
			case "target-digest":
				body, _ := f.baseline.Encode()
				var wire map[string]any
				_ = json.Unmarshal(body, &wire)
				wire["completedAt"] = "2026-11-13T12:00:01.000Z"
				wire["layers"].([]any)[0].(map[string]any)["completedAt"] = wire["completedAt"]
				// Canonical re-encode through the owned codec after an independent wire edit.
				var data baselineData
				edited, _ := json.Marshal(wire)
				_ = json.Unmarshal(edited, &data)
				body, _ = json.Marshal(data)
				f.objects.seed("baselines/"+f.baseline.ID()+".json", body)
				code = "history_mismatch"
			case "selected-corrupt":
				f.objects.seed(baselineSelectionName, []byte("private"))
			case "selected-foreign":
				s, _ := NewBaselineSelection(f.lease.manager.environmentID, "other", f.baseline)
				body, _ := s.Encode()
				f.objects.seed(baselineSelectionName, body)
			case "selected-digest":
				f.selectOK(t)
				obj := f.objects.objects[baselineSelectionName]
				var d baselineSelectionData
				_ = json.Unmarshal(obj.Body, &d)
				d.Selected.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				body, _ := json.Marshal(d)
				f.objects.seed(baselineSelectionName, body)
				code = "selection_mismatch"
			case "previous-missing":
				f.selectOK(t)
				f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
				f.configure(t)
				delete(f.objects.objects, "baselines/"+f.baseline.ID()+".json")
				code = "read_failed"
			case "rewind":
				old := f.objects.objects[preparedHistoryName]
				f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
				f.configure(t)
				f.selectOK(t)
				f.objects.seed(preparedHistoryName, old.Body)
				code = "invalid_transition"
			}
			writes := preparationCalls(f.objects, "CREATE", baselineSelectionName) + preparationCalls(f.objects, "REPLACE", baselineSelectionName)
			b, err := f.selector.Select(leaseBudget(t), f.lease)
			liveSelectionFailure(t, b, err, code)
			if preparationCalls(f.objects, "CREATE", baselineSelectionName)+preparationCalls(f.objects, "REPLACE", baselineSelectionName) != writes {
				t.Fatal("invalid state changed selection")
			}
		})
	}
}
