package reconciliation

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestBaselineExactByteLimitAndOneByteOverflow(t *testing.T) {
	regions := []string{"us-ashburn-1", "us-phoenix-1"}
	id := "boundary"
	small := buildBaseline(t, id, baselineResult(t, nil, nil, nil, regions))
	remaining := baselineLimit - len(baselineWire(t, small))
	if remaining%2 != 0 {
		id += "x"
		remaining--
	}
	compartment := baselineCompartment + strings.Repeat("a", remaining/2)
	r := baselineResult(t, nil, nil, []string{compartment}, regions)
	exact := buildBaseline(t, id, r)
	body := baselineWire(t, exact)
	if len(body) != baselineLimit {
		t.Fatalf("fixture has %d bytes", len(body))
	}
	reloaded, err := DecodeBaseline(body, exact.EnvironmentID(), exact.ID())
	if err != nil || !reloaded.Covers(r) {
		t.Fatal("exact limit rejected", err)
	}
	got, err := r.Baseline(id + "x")
	baselineFailure(t, got, err, "too_large")
	withPair := baselineResult(t, []notices.Notice{candidateNotice(t, 1, nil, true)}, nil, []string{compartment}, regions)
	got, err = withPair.Baseline(id)
	baselineFailure(t, got, err, "too_large")
	tooMuchScope := baselineResult(t, nil, nil, []string{compartment + strings.Repeat("a", 1000)}, regions)
	got, err = tooMuchScope.Baseline(id)
	baselineFailure(t, got, err, "too_large")
	got, err = DecodeBaseline(append(body, ' '), exact.EnvironmentID(), exact.ID())
	baselineFailure(t, got, err, "too_large")
	old := buildBaseline(t, "old", baselineResult(t, nil, nil, []string{compartment}, []string{"us-ashburn-1"}))
	before := baselineWire(t, old)
	got, err = old.Extend(id, r)
	baselineFailure(t, got, err, "too_large")
	if !bytes.Equal(before, baselineWire(t, old)) {
		t.Fatal("overflow changed prior history")
	}
}

func TestBaselineSmallCellsDoNotInheritNoticeRecordArrayCap(t *testing.T) {
	compartments := make([]string, 9000)
	for i := range compartments {
		compartments[i] = fmt.Sprintf("ocid1.compartment.oc1..c%04d", i)
	}
	r := baselineResult(t, nil, nil, compartments, nil)
	b := buildBaseline(t, "small-cells", r)
	if len(b.Layers()[0].Cells) != len(compartments) {
		t.Fatal("scope truncated")
	}
	body := baselineWire(t, b)
	if len(body) > baselineLimit {
		t.Fatal("fixture exceeds byte cap")
	}
	decoded, err := DecodeBaseline(body, b.EnvironmentID(), b.ID())
	if err != nil || !decoded.Covers(r) {
		t.Fatal("valid small-cell array rejected", err)
	}
}

func TestBaselineExcessiveCartesianScopeFailsBeforeExpansion(t *testing.T) {
	compartments := make([]string, 20000)
	for i := range compartments {
		compartments[i] = fmt.Sprintf("ocid1.compartment.oc1..c%05d", i)
	}
	r := baselineResult(t, nil, nil, compartments, []string{"us-ashburn-1", "us-phoenix-1"})
	got, err := r.Baseline("too-many")
	baselineFailure(t, got, err, "too_large")
	base := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	if base.Covers(r) {
		t.Fatal("bounded baseline claims excessive scope")
	}
	got, err = base.Extend("expanded", r)
	baselineFailure(t, got, err, "too_large")
}
