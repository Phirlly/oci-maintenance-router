package reconciliation

import (
	"context"
	"strings"
	"testing"
)

func TestSetupSelectorRejectsMissingConfirmationAndUnboundRelease(t *testing.T) {
	f := setupConfirmedRuntime(t)
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
	f.objects.seed("configuration/router.json", cfg)
	if s, err := NewLiveBaselineSelector(cfg, []byte(preparationCatalog), candidateLocation(), "bad-release", f.objects, operatorClient(t)); err == nil || s != nil {
		t.Fatal("unbound release selected")
	}
	if s, err := NewLiveBaselineSelector(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, nil); err == nil || s != nil {
		t.Fatal("missing channel accepted")
	}
	s, h := selectedSetupFor(t, f)
	delete(f.objects.objects, "control/setup-confirmations/"+s.installation+"/"+h.record.data.Confirmation.ID+".json")
	selector, err := NewLiveBaselineSelector(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	selector.setup.read = func(context.Context) (OperatorChannel, error) {
		t.Fatal("missing proof reached channel lookup")
		return OperatorChannel{}, nil
	}
	b, err := selector.Select(leaseBudget(t), f.lease)
	detail := liveSelectionFailure(t, b, err, "read_failed")
	if detail.HTTPStatus != 404 || detail.WriteUncertain {
		t.Fatal("missing confirmation provenance lost", detail)
	}
}

func TestSetupReporterRejectsLegacyConfigurationWithoutOperatorAuthority(t *testing.T) {
	f := setupConfirmedRuntime(t)
	cfg := preparationConfig(t, func(v map[string]any) {
		v["schemaVersion"], v["mode"], v["activeBaselineId"] = 1, "LIVE", "baseline"
		delete(v, "installationId")
		delete(v, "preparation")
	})
	if r, err := NewIssueReporter(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t)); err == nil || r != nil {
		t.Fatal("legacy display-only LIVE became reporting authority")
	}
}
