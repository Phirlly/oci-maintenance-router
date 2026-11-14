package reconciliation

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/notices"
)

func baselineWire(t *testing.T, b Baseline) []byte {
	t.Helper()
	body, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func alteredBaseline(t *testing.T, b Baseline, change func(*baselineData)) []byte {
	t.Helper()
	var data baselineData
	if err := json.Unmarshal(baselineWire(t, b), &data); err != nil {
		t.Fatal(err)
	}
	change(&data)
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestBaselineCanonicalGoldenAndRestart(t *testing.T) {
	r := baselineResult(t, nil, nil, []string{"ocid1.tenancy.oc1..example", baselineCompartment}, []string{"us-phoenix-1", "us-ashburn-1"})
	b := buildBaseline(t, "initial", r)
	want, err := os.ReadFile("testdata/empty-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baselineWire(t, b), want) {
		t.Fatal("baseline canonical encoding drift")
	}
	decoded, err := DecodeBaseline(want, b.EnvironmentID(), b.ID())
	if err != nil {
		t.Fatal("valid baseline rejected", err)
	}
	if !reflect.DeepEqual(b.Layers(), decoded.Layers()) || !decoded.Covers(r) || !bytes.Equal(baselineWire(t, decoded), want) {
		t.Fatal("restart changed baseline")
	}
	clear(want)
	if !decoded.Covers(r) {
		t.Fatal("decoder retained caller bytes")
	}
}

func TestBaselineDecoderRejectsMalformedOrAmbiguousWire(t *testing.T) {
	b := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	wire := string(baselineWire(t, b))
	for _, bad := range []string{
		"", "null", "[]", "{}", wire + "{}", wire + " ",
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"unknown":"private diagnostic"`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"SchemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1.0`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":"1"`, 1),
		strings.Replace(wire, `"layers":[`, `"layers":[null,`, 1),
		strings.Replace(wire, `"cells":[`, `"cells":[null,`, 1),
		strings.Replace(wire, `"revisions":[]`, `"revisions":null`, 1),
		strings.Replace(wire, `"revisions":[]`, `"revisions":[null]`, 1),
		strings.Replace(wire, `"initial"`, `"\ud800"`, 1),
		strings.Replace(wire, `"initial"`, "\"\xff\"", 1),
		`{"layers":` + strings.Repeat("[", 33) + "null" + strings.Repeat("]", 33) + "}",
		`{"layers":[` + strings.Repeat("null,", baselineEntries) + "null]}",
		`{"layers"`, `{"layers":}`, `{"layers":[}`, `{"layers":[{"broken":]}`, `{"layers":[[]`,
	} {
		got, err := DecodeBaseline([]byte(bad), b.EnvironmentID(), b.ID())
		if err == nil || got.ID() != "" || got.Layers() != nil {
			t.Fatal("invalid baseline returned partial state")
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("raw data escaped error")
		}
	}
}

func TestBaselineDecoderBindsIdentityAndMeaning(t *testing.T) {
	a, z := candidateNotice(t, 1, nil, true), candidateNotice(t, 2, nil, true)
	b := buildBaseline(t, "initial", baselineResult(t, []notices.Notice{z, a}, nil, []string{baselineOtherCompartment, baselineCompartment}, nil))
	for _, change := range []func(*baselineData){
		func(d *baselineData) { d.SchemaVersion = 2 }, func(d *baselineData) { d.EnvironmentID = "invalid" }, func(d *baselineData) { d.BaselineID = "../wrong" },
		func(d *baselineData) { d.CompletedAt = "invalid" }, func(d *baselineData) { d.CompletedAt = "2026-11-13T12:00:01.000Z" },
		func(d *baselineData) { d.Layers = nil }, func(d *baselineData) { d.Layers[0].CompletedAt = "2026-11-13T12:00:01.000Z" }, func(d *baselineData) { d.Layers[0].CompletedAt = "invalid" },
		func(d *baselineData) { d.Layers[0].Cells = nil }, func(d *baselineData) { d.Layers[0].Cells[0].Region = "bad region" }, func(d *baselineData) { d.Layers[0].Cells[0].CompartmentID = "bad compartment" },
		func(d *baselineData) {
			d.Layers[0].Cells[0], d.Layers[0].Cells[1] = d.Layers[0].Cells[1], d.Layers[0].Cells[0]
		},
		func(d *baselineData) { d.Layers[0].Cells[1] = d.Layers[0].Cells[0] }, func(d *baselineData) { d.Layers = append(d.Layers, d.Layers[0]) },
		func(d *baselineData) { d.Layers[0].Revisions[0].NoticeID = "invalid" }, func(d *baselineData) { d.Layers[0].Revisions[0].Digest = strings.Repeat("G", 64) },
		func(d *baselineData) {
			d.Layers[0].Revisions[0], d.Layers[0].Revisions[1] = d.Layers[0].Revisions[1], d.Layers[0].Revisions[0]
		},
		func(d *baselineData) { d.Layers[0].Revisions[1] = d.Layers[0].Revisions[0] },
	} {
		got, err := DecodeBaseline(alteredBaseline(t, b, change), b.EnvironmentID(), b.ID())
		if err == nil || got.ID() != "" {
			t.Fatal("invalid invariant accepted")
		}
	}
	for _, identity := range [][2]string{{strings.Repeat("f", 64), b.ID()}, {b.EnvironmentID(), "other"}} {
		got, err := DecodeBaseline(baselineWire(t, b), identity[0], identity[1])
		baselineFailure(t, got, err, "identity_mismatch")
	}
}
