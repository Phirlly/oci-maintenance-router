package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestPreparationConstructorRejectsUntrustedInputs(t *testing.T) {
	_, s, _ := preparationFixture(t, nil)
	var nilObjects *preparationObjects
	var nilAnnouncements *announcementDetails
	for _, tc := range []struct {
		config, catalog []byte
		location        configuration.LedgerLocation
		source          AnnouncementSource
		objects         PreparationObjects
	}{
		{[]byte(candidateConfig), []byte(preparationCatalog), candidateLocation(), announcementDetails{}, s},
		{[]byte("private config"), []byte(preparationCatalog), candidateLocation(), announcementDetails{}, s},
		{preparationConfig(t), []byte("private catalog"), candidateLocation(), announcementDetails{}, s},
		{preparationConfig(t), []byte(preparationCatalog), configuration.LedgerLocation{}, announcementDetails{}, s},
		{preparationConfig(t), []byte(preparationCatalog), candidateLocation(), nil, s},
		{preparationConfig(t), []byte(preparationCatalog), candidateLocation(), nilAnnouncements, s},
		{preparationConfig(t), []byte(preparationCatalog), candidateLocation(), announcementDetails{}, nil},
		{preparationConfig(t), []byte(preparationCatalog), candidateLocation(), announcementDetails{}, nilObjects},
	} {
		before := len(s.calls)
		p, err := NewBaselinePreparation(tc.config, tc.catalog, tc.location, tc.source, tc.objects)
		if p != nil {
			t.Fatal("invalid constructor returned coordinator")
		}
		preparationFailed(t, Baseline{}, err, "invalid_request")
		if len(s.calls) != before {
			t.Fatal("constructor made I/O")
		}
	}
}

func TestPreparationRejectsInvalidLifetimeOrAuthorityBeforeIO(t *testing.T) {
	for _, tc := range []string{"nil-owner", "zero-owner", "nil-lease", "zero-lease", "foreign", "nil-context", "unbounded", "canceled", "paused"} {
		t.Run(tc, func(t *testing.T) {
			p, s, l := preparationFixture(t, nil)
			ctx := leaseBudget(t)
			code := "invalid_request"
			switch tc {
			case "nil-owner":
				p = nil
			case "zero-owner":
				p = &BaselinePreparation{}
			case "nil-lease":
				l = nil
			case "zero-lease":
				l = &Lease{}
			case "foreign":
				_, _, l = discoveryFixture(t, nil, nil)
				copyManager := *l.manager
				copyManager.environmentID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				l.manager = &copyManager
				code = "environment_mismatch"
			case "nil-context":
				ctx = nil
			case "unbounded":
				ctx = context.Background()
				code = "deadline_required"
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				code = "canceled"
			case "paused":
				p = preparationFor(t, s, preparationConfig(t, func(c map[string]any) { c["mode"] = "PAUSED" }), nil)
				code = "intent_required"
			}
			before := len(s.calls)
			b, err := p.Prepare(ctx, l)
			preparationFailed(t, b, err, code)
			err = p.Initialize(ctx, l)
			preparationFailed(t, Baseline{}, err, code)
			if len(s.calls) != before {
				t.Fatal("invalid authority made I/O")
			}
		})
	}
}

func TestPreparationOwnsInputBytesAndInitializeFalseAllowsExistingHistory(t *testing.T) {
	p, s, l := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), l); err != nil {
		t.Fatal(err)
	}
	cfg := preparationConfig(t, func(c map[string]any) { c["preparation"] = map[string]any{"initialize": false} })
	cat := []byte(preparationCatalog)
	s.seed("configuration/router.json", cfg)
	p, err := NewBaselinePreparation(cfg, cat, candidateLocation(), announcementDetails{}, s)
	if err != nil {
		t.Fatal(err)
	}
	clear(cfg)
	clear(cat)
	b := prepareSuccessfully(t, p, l)
	layers := b.Layers()
	layers[0].Cells[0].Region = "mutated"
	if b.Layers()[0].Cells[0].Region == "mutated" {
		t.Fatal("caller mutated prepared evidence")
	}
}
