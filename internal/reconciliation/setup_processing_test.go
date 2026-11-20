package reconciliation

import (
	"context"
	"strings"
	"testing"
)

// Keep the existing synthetic service adapters, but use genuine schema8 test
// records, confirmation, selection and processing against their shared store.
func setupOnProcessingFixture(t *testing.T, p *processingFixture) *setupRuntimeFixture {
	t.Helper()
	f := &setupRuntimeFixture{objects: p.live.objects, lease: p.live.lease}
	f.configure(t, "SEND_TEST", 1)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, "CONFIRM", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSetupProcessingUsesSavedProofBeforeEveryAttempt(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "confirmation-changed"}[invalidate], func(t *testing.T) {
			p := processingFixtureFor(t, processingNotice(t, 1, processingDB))
			f := setupOnProcessingFixture(t, p)
			cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
			f.objects.seed("configuration/router.json", cfg)
			old := p.p
			var err error
			p.p, err = NewMaintenanceProcessor(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), p.source, f.objects, operatorClient(t), diagnosticVault(t), processingMySQL(t))
			if err != nil {
				t.Fatal(err)
			}
			p.p.selector.setup.read = f.coordinator.read
			p.p.discovery.wait, p.p.wait, p.p.jitter = old.discovery.wait, old.wait, old.jitter
			p.p.mysql, p.p.current, p.p.version, p.p.senders = old.mysql, old.current, old.version, old.senders
			if invalidate {
				p.p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
					m, err := old.current(ctx, id)
					f.objects.seed("control/setup-current/installation-1.json", []byte("invalid"))
					return m, err
				}
			}
			r, err := p.p.Run(leaseBudget(t), f.lease)
			if invalidate {
				if err == nil || r.Snapshot().Completed || p.sent.Load() != 0 {
					t.Fatal("changed confirmation permitted delivery", err)
				}
			} else if err != nil || !r.Snapshot().Completed || r.Snapshot().Accepted != 1 || p.sent.Load() != 1 {
				t.Fatal("saved confirmation did not enable normal delivery", r.Snapshot(), err)
			}
			if f.diagnosticSends != 1 || f.operatorSends != 1 {
				t.Fatal("processing repeated setup")
			}
		})
	}
}

func TestSetupReporterPublishesEvenWhenDiagnosticRecordUnavailable(t *testing.T) {
	f := issueReporterFixtureFor(t)
	setup := setupOnProcessingFixture(t, f.processing)
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0)
	setup.objects.seed("configuration/router.json", cfg)
	old := f.reporter
	var err error
	f.reporter, err = NewIssueReporter(cfg, []byte(preparationCatalog), candidateLocation(), setup.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	f.reporter.setupProof.read = setup.coordinator.read
	f.reporter.read = old.read
	d, _ := setupRequestIDs(setup.lease.manager.environmentID, "installation-1", 1)
	delete(setup.objects.objects, "control/diagnostics/installation-1/"+d+".json")
	// The earlier credential issue belongs to the prior configuration. Use a
	// current durable storage problem to exercise publication after the change.
	setup.objects.seed(reportNoticeName(noticeID(1)), []byte("unreadable saved notice"))
	got, err := f.reporter.Refresh(leaseBudget(t), setup.lease, f.snapshot)
	if err != nil || !got.Accepted || !got.Dispatched || f.sends != 1 {
		t.Fatal("diagnostic failure disabled reporting", got, err)
	}
	if f.processing.sent.Load() != 0 {
		t.Fatal("reporting sent maintenance")
	}
}
