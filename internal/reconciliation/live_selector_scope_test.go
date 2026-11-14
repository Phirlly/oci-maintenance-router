package reconciliation

import (
	"bytes"
	"testing"
)

func TestLiveSelectorRequiresPreparedCoverageForExactScope(t *testing.T) {
	for _, scope := range [][]string{{baselineCompartment, baselineOtherCompartment}, {"ocid1.tenancy.oc1..example"}} {
		f := liveSelectorFixtureFor(t)
		f.configure(t, func(v map[string]any) { v["compartmentIds"] = scope })
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, "scope_unprepared")
		if preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
			t.Fatal("uncovered scope selected")
		}
	}
}

func (f *liveSelectorFixture) extend(t *testing.T, compartments, regions []string) Baseline {
	t.Helper()
	cfg := preparationConfig(t, func(v map[string]any) {
		if compartments != nil {
			v["compartmentIds"] = compartments
		}
		if regions != nil {
			m := map[string]string{}
			for _, r := range regions {
				if r == "us-ashburn-1" {
					m[r] = "iad"
				} else {
					m[r] = "phx"
				}
			}
			v["regionMap"] = m
		}
	})
	p := preparationFor(t, f.objects, cfg, nil)
	return prepareSuccessfully(t, p, f.lease)
}

func TestLiveSelectorAdvancesAcrossSeveralPreparedLayersWithoutRebaselining(t *testing.T) {
	for _, initial := range []bool{false, true} {
		f := liveSelectorFixtureFor(t)
		if !initial {
			f.selectOK(t)
		}
		f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
		latest := f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, []string{"us-ashburn-1", "us-phoenix-1"})
		if !initial {
			obj, _ := f.objects.Get(leaseBudget(t), baselineSelectionName)
			s, err := DecodeBaselineSelection(obj.Body, f.lease.manager.environmentID, "installation-1")
			if err != nil || !s.Matches(f.baseline) {
				t.Fatal("PREVIEW changed selection")
			}
		}
		f.configure(t, func(v map[string]any) {
			v["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment}
			v["regionMap"] = map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"}
		})
		b := f.selectOK(t)
		want, _ := latest.Encode()
		got, _ := b.Encode()
		if !bytes.Equal(want, got) || len(b.Layers()) != 3 {
			t.Fatal("did not preserve complete cumulative baseline")
		}
		writes := preparationCalls(f.objects, "REPLACE", baselineSelectionName)
		if initial && writes != 0 || !initial && writes != 1 {
			t.Fatal("wrong conditional transition")
		}
		f.configure(t)
		f.selectOK(t)
		f.configure(t, func(v map[string]any) { v["compartmentIds"] = []string{baselineCompartment, baselineOtherCompartment} })
		f.selectOK(t)
		if preparationCalls(f.objects, "REPLACE", baselineSelectionName) != writes {
			t.Fatal("scope removal/readdition rebaselined")
		}
	}
}
