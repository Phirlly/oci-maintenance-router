package reconciliation

import (
	"bytes"
	"strings"
	"testing"
)

func TestBaselineSelectionRejectsInvalidCanonicalRecords(t *testing.T) {
	_, _, first, _ := preparedStoreFixture(t)
	s := selectionFor(t, first)
	body, _ := s.Encode()
	wire := string(body)
	for _, bad := range []string{
		"", "null", "{}", "[]", wire + " ", wire + "{}", strings.Repeat(" ", 4097),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":2`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1.0`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"SchemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(wire, `"schemaVersion":1`, `"schemaVersion":1,"private":"details"`, 1),
		strings.Replace(wire, `"installation-1"`, `"../private"`, 1),
		strings.Replace(wire, `"installation-1"`, `"\ud800"`, 1),
		strings.Replace(wire, `"installation-1"`, string([]byte{34, 255, 34}), 1),
		strings.Replace(wire, `"selectedBaseline":`, `"other":`, 1),
		strings.Replace(wire, `{"baselineId":"initial","contentSha256":"`+s.BaselineDigest()+`"}`, "null", 1),
		strings.Replace(wire, s.BaselineDigest(), "bad", 1),
		strings.Replace(wire, `"baselineId":"initial"`, `"baselineId":"../bad"`, 1),
		strings.Replace(wire, `"baselineId":"initial"`, `"baselineId":"initial","unknown":true`, 1),
		`{"unknown":` + strings.Repeat("[", 1000) + "null" + strings.Repeat("]", 1000) + "}",
	} {
		got, err := DecodeBaselineSelection([]byte(bad), s.EnvironmentID(), s.InstallationID())
		if err == nil || got.EnvironmentID() != "" || got.InstallationID() != "" || got.BaselineID() != "" || got.BaselineDigest() != "" || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid selection accepted or leaked input", err)
		}
	}
	for _, identity := range [][2]string{{strings.Repeat("f", 64), s.InstallationID()}, {s.EnvironmentID(), "other"}} {
		if got, err := DecodeBaselineSelection(body, identity[0], identity[1]); err == nil || got.EnvironmentID() != "" {
			t.Fatal("foreign selection admitted")
		}
	}
	if !bytes.Equal(body, []byte(wire)) {
		t.Fatal("decoder mutated input")
	}
}

func TestBaselineSelectionInvalidInputsAndZeroValues(t *testing.T) {
	_, _, first, _ := preparedStoreFixture(t)
	for _, identity := range [][2]string{{"", "install"}, {strings.Repeat("A", 64), "install"}, {first.EnvironmentID(), ""}, {first.EnvironmentID(), "../id"}, {first.EnvironmentID(), strings.Repeat("a", 129)}, {strings.Repeat("f", 64), "install"}} {
		if got, err := NewBaselineSelection(identity[0], identity[1], first); err == nil || got.EnvironmentID() != "" {
			t.Fatal("invalid identity admitted")
		}
	}
	if got, err := NewBaselineSelection(first.EnvironmentID(), "install", Baseline{}); err == nil || got.EnvironmentID() != "" {
		t.Fatal("empty baseline admitted")
	}
	if got, err := NewBaselineSelection(first.EnvironmentID(), strings.Repeat("a", 128), first); err != nil || len(got.InstallationID()) != 128 {
		t.Fatal("valid maximum installation rejected")
	}
	if body, err := (BaselineSelection{}).Encode(); body != nil || err == nil {
		t.Fatal("zero state encoded")
	}
	if (BaselineSelection{}).Matches(first) {
		t.Fatal("zero selection matched")
	}
}
