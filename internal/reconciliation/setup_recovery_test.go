package reconciliation

import (
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
)

func TestSetupRecoveryReviewerAcceptsCodeFreeLiveControls(t *testing.T) {
	p := processingFixtureFor(t)
	saved, err := ocistorage.NewNoticeStore(p.live.objects, p.live.lease.manager.environmentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewRecoveryReviewer(setupRuntimeConfig(t, "LIVE", "NONE", 0), []byte(preparationCatalog), candidateLocation(), p.source, saved, processingMySQL(t)); err != nil {
		t.Fatal("review rejected schema8", err)
	}
}

func TestSetupReadinessFailuresAreControlledForRecovery(t *testing.T) {
	for _, code := range []string{"confirmation_required", "setup_stale", "confirmation_mismatch"} {
		for _, uncertain := range []bool{false, true} {
			for _, status := range []int{0, 403, 500} {
				e := &SetupError{Code: code, HTTPStatus: status, WriteUncertain: uncertain}
				mapped := liveSelectionError(nil, e)
				if mapped.Code != code || mapped.HTTPStatus != status || mapped.WriteUncertain != uncertain {
					t.Fatal("owned failure lost", mapped)
				}
				if missingRecoveryReadiness(mapped) != (status == 0 && !uncertain) {
					t.Fatal("wrong recovery disposition", mapped)
				}
			}
		}
	}
	for _, code := range []string{"read_failed", "invalid_record", "selection_changed", strings.Repeat("a", 64)} {
		if missingRecoveryReadiness(liveSelectionError(nil, &SetupError{Code: code})) {
			t.Fatal("storage failure treated as unavailable readiness", code)
		}
	}
}
