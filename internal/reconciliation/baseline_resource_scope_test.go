package reconciliation

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"oci-maintenance-router/internal/notices"
)

// Typed scope is synthetic until configuration and Compute discovery are adopted.
// The real completed-discovery fixture still supplies the revision evidence.
func resourceBaselineResult(t *testing.T, types []string, ns ...notices.Notice) DiscoveryResult {
	t.Helper()
	r := baselineResult(t, ns, nil, nil, nil)
	r.scope.resourceTypes = slices.Clone(types)
	return r
}

func TestBaselineLegacyCoverageIsOnlyMySQL(t *testing.T) {
	n := candidateNotice(t, 1, nil, true)
	b := buildBaseline(t, "legacy", baselineResult(t, []notices.Notice{n}, nil, nil, nil))
	if !b.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, n.Key()) {
		t.Fatal("legacy MySQL suppression changed")
	}
	for _, kind := range []string{"instance", "", "volume", "MySQL"} {
		if b.Suppresses(kind, "us-ashburn-1", baselineCompartment, n.Key()) {
			t.Fatal("another resource type inherited MySQL suppression", kind)
		}
	}
	if b.Covers(resourceBaselineResult(t, []string{"instance"}, n)) {
		t.Fatal("legacy MySQL history covered an unprepared instance scope")
	}
}

func TestBaselineResourceExpansionPreservesEarlierEligibility(t *testing.T) {
	old, arrived := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	base := buildBaseline(t, "legacy", baselineResult(t, []notices.Notice{old}, nil, nil, nil))
	before := baselineWire(t, base)
	r := resourceBaselineResult(t, []string{"instance", "mysqldbsystem"}, old, arrived)
	next, err := base.Extend("with-instances", r)
	if err != nil {
		t.Fatal("new resource type was not prepared", err)
	}
	if !bytes.Equal(before, baselineWire(t, base)) || !reflect.DeepEqual(base.Layers()[0], next.Layers()[0]) || len(next.Layers()) != 2 {
		t.Fatal("typed expansion rewrote earlier history")
	}
	if !next.Covers(r) || next.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, arrived.Key()) || !next.Suppresses("instance", "us-ashburn-1", baselineCompartment, arrived.Key()) {
		t.Fatal("resource coverage mixed independent snapshot eligibility")
	}
	for _, where := range [][2]string{{"us-phoenix-1", baselineCompartment}, {"us-ashburn-1", baselineOtherCompartment}} {
		if next.Suppresses("instance", where[0], where[1], arrived.Key()) {
			t.Fatal("typed suppression escaped exact location")
		}
	}
	for _, types := range [][]string{{"instance"}, {"mysqldbsystem"}, {"instance", "mysqldbsystem"}} {
		reused := resourceBaselineResult(t, types, arrived)
		if !next.Covers(reused) {
			t.Fatal("covered resource type was forgotten")
		}
		got, err := next.Extend("must-not-rebaseline", reused)
		baselineFailure(t, got, err, "no_new_scope")
	}
	decoded, err := DecodeBaseline(baselineWire(t, next), next.EnvironmentID(), next.ID())
	if err != nil || !decoded.Covers(r) || !decoded.Suppresses("instance", "us-ashburn-1", baselineCompartment, arrived.Key()) {
		t.Fatal("restart lost typed coverage", err)
	}
	selection := selectionFor(t, base)
	selected, err := selection.Advance(base, decoded)
	if err != nil || !selected.Matches(decoded) {
		t.Fatal("typed extension broke selected-history ancestry", err)
	}
	h, _ := NewPreparedHistory(base.EnvironmentID(), "installation-1")
	h = advancePrepared(t, h, Baseline{}, base)
	if nextHead := advancePrepared(t, h, base, decoded); nextHead.BaselineID() != next.ID() {
		t.Fatal("typed extension broke prepared-history ancestry")
	}
}

func TestBaselineStillCapturesRelatedNonmaintenanceMySQL(t *testing.T) {
	n := normalizeCandidate(t, notices.Record{NoticeID: noticeID(1), AnnouncementType: "INFORMATION", Services: []string{"Oracle HeatWave MySQL"}})
	b := buildBaseline(t, "information", baselineResult(t, []notices.Notice{n}, nil, nil, nil))
	if !b.Suppresses("mysqldbsystem", "us-ashburn-1", baselineCompartment, n.Key()) {
		t.Fatal("nonmaintenance candidate was dropped from legacy history")
	}
}

func TestBaselineMySQLAdditionRetainsTypedHistory(t *testing.T) {
	first := buildBaseline(t, "instances", resourceBaselineResult(t, []string{"instance"}))
	r := resourceBaselineResult(t, []string{"instance", "mysqldbsystem"})
	next, err := first.Extend("both", r)
	if err != nil || !next.Covers(r) || !reflect.DeepEqual(first.Layers()[0], next.Layers()[0]) || next.data.SchemaVersion != 2 {
		t.Fatal("MySQL addition lost existing typed history", err)
	}
}

func TestBaselineAncestryRejectsReassignedResourceTypes(t *testing.T) {
	first := buildBaseline(t, "legacy", baselineResult(t, nil, nil, nil, nil))
	next, err := first.Extend("expanded", resourceBaselineResult(t, []string{"instance", "mysqldbsystem"}))
	if err != nil {
		t.Fatal(err)
	}
	wire := alteredBaseline(t, next, func(d *baselineData) {
		d.Layers[0].Cells[0].ResourceType = "instance"
		d.Layers[1].Cells[0].ResourceType = ""
	})
	reassigned, err := DecodeBaseline(wire, next.EnvironmentID(), next.ID())
	if err != nil {
		t.Fatal("fixture must be a valid standalone baseline", err)
	}
	h, _ := NewPreparedHistory(first.EnvironmentID(), "installation-1")
	h = advancePrepared(t, h, Baseline{}, first)
	if got, err := h.Advance(first, reassigned); err == nil || got.BaselineID() != "" {
		t.Fatal("prepared ancestry reassigned resource history")
	}
	s := selectionFor(t, first)
	if got, err := s.Advance(first, reassigned); err == nil || got.BaselineID() != "" {
		t.Fatal("selected ancestry reassigned resource history")
	}
}
