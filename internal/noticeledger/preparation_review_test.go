package noticeledger

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestPreparationReviewTracksAssignmentsAndChangedContext(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_unavailable", clock(), 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	config := preparationConfigDigest()
	view, err := r.Review(p, config)
	if err != nil || len(view.Preparation) != 2 || view.Preparation[0].Status != "LAST_BLOCKED" || len(view.Issues) != 0 {
		t.Fatal("matching saved evidence not visible", view, err)
	}
	changed, err := r.Review(p, strings.Repeat("b", 64))
	if err != nil || changed.Preparation[0].Status != "RECHECK_REQUIRED" {
		t.Fatal("configuration change marked resolved", err)
	}
	held, err := r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	view, err = held.Review(p, config)
	if err != nil || view.Preparation[0].Status != "RECHECK_REQUIRED" || len(view.Issues) == 0 {
		t.Fatal("hold hidden", view, err)
	}
	prepared, err := r.Prepare(clock(), preparations(t, source(db, otherDB), db, otherDB)...)
	if err != nil {
		t.Fatal(err)
	}
	view, err = prepared.Review(p, config)
	if err != nil || view.Preparation[0].Status != "HISTORICAL" || view.Preparation[1].Status != "HISTORICAL" {
		t.Fatal("prepared observations still current", err)
	}
	if len(p.Snapshot().Observations) != 2 {
		t.Fatal("history removed")
	}
	view.Preparation[0].Observation.Code = "changed"
	if p.Snapshot().Observations[0].Code != "secret_unavailable" {
		t.Fatal("view aliases retained evidence")
	}
}

func TestSavedReviewSuppressesTerminalNoticeHolds(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "state_capacity", clock(), 0))
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	r = recoveryRecord(t, r)
	q := RecoveryRequest{RequestID: "close-review", Kind: CloseRevision, TargetID: r.Snapshot().Revisions[0].Digest, RoutingGeneration: r.Snapshot().Revisions[0].RoutingGeneration}
	q.ReviewFingerprint, err = r.ReviewFingerprint(q.Kind, q.TargetID, q.RoutingGeneration, "")
	if err != nil {
		t.Fatal(err)
	}
	r, action, err := r.CloseRecovery(q, clock())
	if err != nil || action.Status != RecoveryClosed {
		t.Fatal("close fixture", err)
	}
	view, err := r.Review(p, preparationConfigDigest())
	if err != nil || len(view.Issues) != 0 || view.Preparation[0].Status != "HISTORICAL" {
		t.Fatal("closed work still actionable", view, err)
	}
}

func TestSavedReviewDerivesUnresolvedRowsWithoutSidecar(t *testing.T) {
	r := observed(t, source(db, otherDB))
	view, err := r.Review(PreparationIssues{}, strings.Repeat("a", 64))
	if err != nil || len(view.Issues) != 2 || view.Issues[0].Kind != "RESOURCE" || view.Issues[0].Code != "lookup_missing" || view.Issues[0].ResourceIndex != 0 || view.Issues[1].ResourceIndex != 1 {
		t.Fatal("unresolved references missing", view, err)
	}
}
