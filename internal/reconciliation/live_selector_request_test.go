package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

func TestLiveSelectorRequiresExplicitLIVEAndCompleteOwnedInputs(t *testing.T) {
	_, objects, _ := preparationFixture(t, nil)
	before := len(objects.calls)
	valid := liveSchemaConfig(t, "LIVE")
	for _, tc := range []struct {
		name            string
		config, catalog []byte
		location        configuration.LedgerLocation
		release         string
		objects         BaselineSelectionObjects
		client          *ocinotifications.Client
		code            string
	}{
		{"valid", valid, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, operatorClient(t), ""},
		{"bad-config", []byte("private"), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, operatorClient(t), "invalid_request"},
		{"bad-catalog", valid, []byte("private"), candidateLocation(), strings.Repeat("b", 64), objects, operatorClient(t), "invalid_request"},
		{"location", valid, []byte(preparationCatalog), configuration.LedgerLocation{}, strings.Repeat("b", 64), objects, operatorClient(t), "invalid_request"},
		{"release", valid, []byte(preparationCatalog), candidateLocation(), "private", objects, operatorClient(t), "invalid_request"},
		{"nil-objects", valid, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), nil, operatorClient(t), "invalid_request"},
		{"typed-nil", valid, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), (*preparationObjects)(nil), operatorClient(t), "invalid_request"},
		{"nil-client", valid, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, nil, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewLiveBaselineSelector(tc.config, tc.catalog, tc.location, tc.release, tc.objects, tc.client)
			if tc.code == "" {
				if err != nil || s == nil || s.diagnostic == nil || s.operator == nil {
					t.Fatal("valid selector rejected", err)
				}
			} else {
				liveSelectionFailure(t, Baseline{}, err, tc.code)
				if s != nil {
					t.Fatal("invalid constructor returned selector")
				}
			}
		})
	}
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		s, err := NewLiveBaselineSelector(liveSchemaConfig(t, mode), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, operatorClient(t))
		liveSelectionFailure(t, Baseline{}, err, "intent_required")
		if s != nil {
			t.Fatal("non-LIVE selector accepted")
		}
	}
	old := diagnosticConfig(t, operatorTestConfig)
	_, err := NewLiveBaselineSelector(old, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, operatorClient(t))
	liveSelectionFailure(t, Baseline{}, err, "invalid_request")
	if len(objects.calls) != before {
		t.Fatal("constructor performed I/O")
	}
}
