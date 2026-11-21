package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func selectedProcessingFixture(t *testing.T, kinds []string, initial []notices.Notice, current ...notices.Notice) (*processingFixture, *setupRuntimeFixture) {
	t.Helper()
	f := processingFixtureWithBaseline(t, initial, current...)
	p := resourcePreparation(t, f.live.objects, initial, kinds...)
	prepareSuccessfully(t, p, f.live.lease)
	setup := &setupRuntimeFixture{objects: f.live.objects, lease: f.live.lease, resourceTypes: kinds}
	setup.configure(t, "SEND_TEST", 1)
	var err error
	setup.result, err = setup.coordinator.Run(leaseBudget(t), setup.lease)
	if err != nil {
		t.Fatal(err)
	}
	setup.configure(t, "CONFIRM", 1)
	if _, err = setup.coordinator.Run(leaseBudget(t), setup.lease); err != nil {
		t.Fatal(err)
	}
	configureSelectedProcessing(t, f, setup, kinds)
	return f, setup
}

func configureSelectedProcessing(t *testing.T, f *processingFixture, setup *setupRuntimeFixture, kinds []string, changes ...func(map[string]any)) {
	t.Helper()
	cfg := resourceConfig(t, setupRuntimeConfig(t, "LIVE", "NONE", 0, changes...), kinds...)
	f.live.objects.seed("configuration/router.json", cfg)
	clients := processingMySQL(t)
	clients.Compute = processingCompute(t).Compute
	p, err := NewMaintenanceProcessor(cfg, []byte(resourceCatalogWire), candidateLocation(), strings.Repeat("b", 64), f.source, f.live.objects, operatorClient(t), diagnosticVault(t), clients)
	if err != nil {
		t.Fatal(err)
	}
	p.selector.setup.read = setup.coordinator.read
	old := f.p
	p.discovery.wait, p.wait, p.jitter = old.discovery.wait, old.wait, old.jitter
	lookup := old.mysql["us-ashburn-1"]
	if lookup == nil {
		lookup = old.compute["us-ashburn-1"]
	}
	if p.mysql["us-ashburn-1"] != nil {
		p.mysql["us-ashburn-1"] = lookup
	}
	if p.compute["us-ashburn-1"] != nil {
		p.compute["us-ashburn-1"] = lookup
	}
	p.current, p.version, p.senders = old.current, old.version, old.senders
	f.p, f.live.config, f.live.selector = p, cfg, p.selector
}

func selectedNotice(t *testing.T, number int, category string, ids ...string) notices.Notice {
	t.Helper()
	r := processingNotice(t, number, ids...).Snapshot()
	r.AnnouncementType = category
	r.Services = []string{"Oracle HeatWave MySQL", "Oracle Cloud Infrastructure Compute"}
	return normalizeCandidate(t, r)
}
