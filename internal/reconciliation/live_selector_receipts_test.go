package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLiveSelectorRevalidatesBothSavedReceiptsAndCurrentChannel(t *testing.T) {
	for _, tc := range []struct{ kind, code string }{
		{"diagnostic-code", "receipt_mismatch"}, {"operator-code", "receipt_mismatch"},
		{"diagnostic-record", "invalid_record"}, {"operator-record", "invalid_record"},
		{"diagnostic-unaccepted", "not_accepted"}, {"operator-unaccepted", "not_accepted"},
		{"diagnostic-intent", "identity_conflict"}, {"operator-intent", "identity_conflict"},
		{"channel", "channel_changed"}, {"channel-error", "channel_read"},
		{"release", "identity_conflict"}, {"catalog", "identity_conflict"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f := liveSelectorFixtureFor(t)
			switch tc.kind {
			case "diagnostic-code":
				f.configure(t, func(v map[string]any) {
					v["diagnostic"].(map[string]any)["receiptCode"] = "OMR-00000000000000000000000000000000"
				})
			case "operator-code":
				f.configure(t, func(v map[string]any) {
					v["operatorTest"].(map[string]any)["receiptCode"] = "OMR-OP-00000000000000000000000000000000"
				})
			case "diagnostic-record":
				f.objects.seed(diagnosticObjectName, []byte("private"))
			case "operator-record":
				f.objects.seed(operatorRunObjectName, []byte("private"))
			case "diagnostic-unaccepted":
				at := f.lease.manager.now().Add(-time.Minute)
				r, err := NewDiagnostic(f.selector.diagnostic.input.intent, 7, DiagnosticClaim{strings.Repeat("c", 64), strings.Repeat("d", 64), at, at.Add(10 * time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				body, _ := r.Encode()
				f.objects.seed(diagnosticObjectName, body)
			case "operator-unaccepted":
				at := f.lease.manager.now().Add(-time.Minute)
				r, err := NewOperatorChallenge(f.selector.operator.input.intent, OperatorClaim{strings.Repeat("c", 64), strings.Repeat("d", 64), at, at.Add(10 * time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				body, _ := r.Encode()
				f.objects.seed(operatorRunObjectName, body)
			case "diagnostic-intent":
				f.configure(t, func(v map[string]any) {
					v["diagnostic"].(map[string]any)["secretId"] = "ocid1.vaultsecret.oc1.iad.changed"
				})
			case "operator-intent":
				f.configure(t, func(v map[string]any) { v["operatorTest"].(map[string]any)["endpoint"] = "changed@example.test" })
			case "channel":
				f.selector.operator.read = func(context.Context) (OperatorChannel, error) {
					c := f.selector.operator.input.desired
					c.SubscriptionID = "other"
					return c, nil
				}
			case "channel-error":
				f.selector.operator.read = func(context.Context) (OperatorChannel, error) {
					return OperatorChannel{}, errors.New("private endpoint")
				}
			case "release", "catalog":
				cat, release := []byte(preparationCatalog), strings.Repeat("b", 64)
				if tc.kind == "release" {
					release = strings.Repeat("e", 64)
				} else {
					cat = append(cat, '\n')
					f.objects.seed("configuration/catalog.json", cat)
				}
				s, err := NewLiveBaselineSelector(f.config, cat, candidateLocation(), release, f.objects, operatorClient(t))
				if err != nil {
					t.Fatal(err)
				}
				f.selector = s
			}
			b, err := f.selector.Select(leaseBudget(t), f.lease)
			liveSelectionFailure(t, b, err, tc.code)
			if strings.Contains(err.Error(), "private") || preparationCalls(f.objects, "CREATE", baselineSelectionName) != 0 {
				t.Fatal("invalid readiness leaked data or selected")
			}
		})
	}
}

func TestLiveSelectorOwnsInputBytesAndRoutingChangesDoNotRetest(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.selectOK(t)
	f.configure(t, func(v map[string]any) { v["ownership"].(map[string]any)["key"] = "new-owner" })
	f.selectOK(t)
	cat := []byte(preparationCatalog)
	s, err := NewLiveBaselineSelector(f.config, cat, candidateLocation(), strings.Repeat("b", 64), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	s.operator.read = f.selector.operator.read
	clear(f.config)
	clear(cat)
	b, err := s.Select(leaseBudget(t), f.lease)
	if err != nil || b.ID() != f.baseline.ID() {
		t.Fatal("caller mutation changed pinned selector", err)
	}
	if preparationCalls(f.objects, "REPLACE", baselineSelectionName) != 0 {
		t.Fatal("routing edit reselected baseline")
	}
}
