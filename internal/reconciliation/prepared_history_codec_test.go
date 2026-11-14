package reconciliation

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPreparedHistoryPinsIndependentBaselineDigest(t *testing.T) {
	body, err := os.ReadFile("testdata/empty-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	env := "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"
	b, err := DecodeBaseline(body, env, "initial")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := NewPreparedHistory(env, "installation-1")
	h = advancePrepared(t, h, Baseline{}, b)
	// Independently calculated from the existing synthetic canonical fixture.
	want := `{"schemaVersion":1,"environmentId":"` + env + `","installationId":"installation-1","preparedBaseline":{"baselineId":"initial","contentSha256":"f335e5c382f1c7e7e5850626907e6278d37b68149349f79bb4b374264d95efa6"}}`
	wire, _ := h.Encode()
	if string(wire) != want {
		t.Fatal("reference digest/canonical representation drift")
	}
	got, err := DecodePreparedHistory(wire, env, "installation-1")
	clear(wire)
	again, _ := got.Encode()
	if err != nil || string(again) != want {
		t.Fatal("prepared restart failed", err)
	}
	clear(again)
	if bytes, _ := got.Encode(); string(bytes) != want {
		t.Fatal("encoding exposed state")
	}
}

func TestPreparedHistoryDecoderRejectsAmbiguousAndForeignRecords(t *testing.T) {
	h, _ := NewPreparedHistory(strings.Repeat("a", 64), "installation-1")
	encoded, _ := h.Encode()
	wire := string(encoded)
	for _, bad := range []string{
		"", "null", "{}", "[]", wire + " ", wire + "{}", strings.Repeat(" ", 4097),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":2`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1.0`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"SchemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"private":"details"`, 1),
		strings.Replace(wire, `"installation-1"`, `"../private"`, 1),
		strings.Replace(wire, `"installation-1"`, `"\ud800"`, 1),
		strings.Replace(wire, `"installation-1"`, "\"\xff\"", 1),
		strings.Replace(wire, `,"preparedBaseline":null`, "", 1),
		strings.Replace(wire, `null`, `{}`, 1), strings.Replace(wire, `null`, `[]`, 1),
		strings.Replace(wire, `null`, `{"baselineId":"initial","contentSha256":"bad"}`, 1),
		strings.Replace(wire, `null`, `{"baselineId":"../bad","contentSha256":"`+strings.Repeat("a", 64)+`"}`, 1),
		strings.Replace(wire, `null`, `{"baselineId":"initial","contentSha256":"`+strings.Repeat("a", 64)+`","unknown":true}`, 1),
		`{"unknown":` + strings.Repeat("[", 1000) + "null" + strings.Repeat("]", 1000) + "}",
	} {
		got, err := DecodePreparedHistory([]byte(bad), h.EnvironmentID(), h.InstallationID())
		if err == nil || got.EnvironmentID() != "" || got.InstallationID() != "" || got.BaselineID() != "" || got.BaselineDigest() != "" || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid record admitted or leaked data", err)
		}
	}
	for _, identity := range [][2]string{{strings.Repeat("b", 64), h.InstallationID()}, {h.EnvironmentID(), "other"}} {
		got, err := DecodePreparedHistory(encoded, identity[0], identity[1])
		if err == nil || got.EnvironmentID() != "" {
			t.Fatal("foreign identity admitted")
		}
	}
	if !bytes.Equal(encoded, []byte(wire)) {
		t.Fatal("decoder mutated input")
	}
}

func TestPreparedHistoryAdmissionAndZeroValues(t *testing.T) {
	for _, identity := range [][2]string{{"", "install"}, {strings.Repeat("A", 64), "install"}, {strings.Repeat("a", 64), ""}, {strings.Repeat("a", 64), "../id"}, {strings.Repeat("a", 64), strings.Repeat("a", 129)}} {
		if got, err := NewPreparedHistory(identity[0], identity[1]); err == nil || got.EnvironmentID() != "" {
			t.Fatal("invalid identity accepted")
		}
	}
	if h, err := NewPreparedHistory(strings.Repeat("a", 64), strings.Repeat("z", 128)); err != nil || len(h.InstallationID()) != 128 {
		t.Fatal("valid maximum installation rejected")
	}
	if body, err := (PreparedHistory{}).Encode(); body != nil || err == nil {
		t.Fatal("zero state encoded")
	}
}
