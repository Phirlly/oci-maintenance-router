package reconciliation

import "testing"

func TestSelectedReporterRemainsIndependentOfUnavailableDiagnostic(t *testing.T) {
	f := issueReporterFixtureFor(t)
	setup := &setupRuntimeFixture{objects: f.processing.live.objects, lease: f.processing.live.lease, resourceTypes: []string{"instance"}}
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
	cfg := resourceConfig(t, setupRuntimeConfig(t, "LIVE", "NONE", 0), "instance")
	setup.objects.seed("configuration/router.json", cfg)
	old := f.reporter
	f.reporter, err = NewIssueReporter(cfg, []byte(resourceCatalogWire), candidateLocation(), setup.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	f.reporter.setupProof.read, f.reporter.read = setup.coordinator.read, old.read
	d, _ := setupRequestIDs(setup.lease.manager.environmentID, "installation-1", 1)
	delete(setup.objects.objects, "control/diagnostics/installation-1/"+d+".json")
	setup.objects.seed(reportNoticeName(noticeID(1)), []byte("unreadable saved notice"))
	got, err := f.reporter.Refresh(leaseBudget(t), setup.lease, f.snapshot)
	if err != nil || !got.Accepted || !got.Dispatched || f.sends != 1 || f.processing.sent.Load() != 0 {
		t.Fatal("unavailable diagnostic disabled independent reporting", got, err)
	}
}
