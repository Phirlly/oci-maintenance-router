package reconciliation

import (
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestRecoveryConfigurationConstructorAdoptsIntentWithoutIO(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	for _, present := range []bool{false, true} {
		cfg := liveSchemaConfig(t, "LIVE", func(v map[string]any) {
			recoveryConfiguration(v)
			if !present {
				delete(v, "recovery")
			}
		})
		catalog, err := configuration.DecodeCatalog([]byte(preparationCatalog))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := configuration.Decode(cfg, catalog); err != nil {
			t.Fatal("invalid exclusion fixture", err)
		}
		before := len(f.objects.calls)
		p, err := NewMaintenanceProcessor(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), processingEmptySource(), f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
		if p == nil || err != nil || (p.config.Recovery != nil) != present || len(f.objects.calls) != before || f.diagnosticSends.Load() != 1 || f.operatorSends.Load() != 1 {
			t.Fatal("processor failed to own recovery intent without IO", err)
		}
	}
}

func TestRecoveryConfigurationDiscoveryPreservesNotice3(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	old := candidateRecord(t, n)
	at, err := time.Parse(time.RFC3339Nano, old.Snapshot().UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	r, err := old.MigrateRecovery(at)
	if err != nil {
		t.Fatal(err)
	}
	d, objects, lease := discoveryFixture(t, []notices.Notice{n}, []noticeledger.Record{r})
	cfg := liveSchemaConfig(t, "PREVIEW", recoveryConfiguration)
	current, err := NewDiscovery(cfg, []byte(discoveryCatalog), candidateLocation(), d.collector.announcements, objects)
	if err != nil {
		t.Fatal(err)
	}
	current.wait, current.now = d.wait, d.now
	objects.seed("configuration/router.json", cfg)
	result, err := current.Run(leaseBudget(t), lease)
	if err != nil || result.EnvironmentID() == "" {
		t.Fatal("new configuration rejected notice3", err)
	}
	after := objects.record(t, lease.manager.environmentID, n.Key().NoticeID).Snapshot()
	if after.SchemaVersion != 3 || after.Recovery == nil || len(after.Recovery.Actions) != 0 || len(after.Revisions) != 1 || after.Revisions[0].Digest != n.Key().Digest {
		t.Fatal("discovery executed intent or changed retained history")
	}
}
