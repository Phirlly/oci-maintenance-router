package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
)

func TestBaselineFailedDiscoveryCannotProduceAProposal(t *testing.T) {
	d, _, lease := discoveryFixture(t, nil, nil)
	r, err := d.Run(context.Background(), lease)
	if err == nil {
		t.Fatal("fixture unexpectedly completed")
	}
	got, err := r.Baseline("initial")
	baselineFailure(t, got, err, "invalid_result")
	if bytes, err := (Baseline{}).Encode(); err == nil || bytes != nil {
		t.Fatal("zero baseline encoded")
	}
	if (Baseline{}).Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, notices.RevisionKey{}) {
		t.Fatal("zero baseline suppresses")
	}
}

func TestBaselineRejectsForeignEnvironmentOlderPassAndInvalidExpansionID(t *testing.T) {
	r := baselineResult(t, nil, nil, nil, nil)
	b := buildBaseline(t, "initial", r)
	foreignBytes := alteredBaseline(t, b, func(d *baselineData) { d.EnvironmentID = strings.Repeat("f", 64) })
	foreign, err := DecodeBaseline(foreignBytes, strings.Repeat("f", 64), b.ID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := foreign.Extend("expanded", r)
	baselineFailure(t, got, err, "identity_mismatch")
	if foreign.Covers(r) {
		t.Fatal("foreign baseline covered discovery")
	}
	laterBytes := alteredBaseline(t, b, func(d *baselineData) {
		stamp := baselineTime().Add(time.Second).Format(leaseTimeFormat)
		d.CompletedAt = stamp
		d.Layers[0].CompletedAt = stamp
	})
	later, err := DecodeBaseline(laterBytes, b.EnvironmentID(), b.ID())
	if err != nil {
		t.Fatal(err)
	}
	got, err = later.Extend("expanded", r)
	baselineFailure(t, got, err, "invalid_time")
	got, err = b.Extend("../bad", r)
	baselineFailure(t, got, err, "invalid_id")
}

func TestBaselineRoutingOnlyChangeDoesNotResetHistory(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	old := baselineResult(t, []notices.Notice{n}, nil, nil, nil)
	b := buildBaseline(t, "initial", old)
	changed := baselineResult(t, []notices.Notice{n}, nil, nil, nil, func(c map[string]any) { c["ownership"] = map[string]string{"kind": "freeform", "key": "team"} })
	if !b.Covers(changed) || !b.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, n.Key()) {
		t.Fatal("routing-only change reset baseline")
	}
	got, err := b.Extend("routing-change", changed)
	baselineFailure(t, got, err, "no_new_scope")
}
