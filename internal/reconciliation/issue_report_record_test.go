package reconciliation

import (
	"bytes"
	"strings"
	"testing"
)

const reportTestChannel = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestIssueReportCanonicalRoundTripAndIdentity(t *testing.T) {
	s := newReport(reportTestChannel, "installation-1", strings.Repeat("b", 64))
	body, err := encodeReport(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeReport(body, s.Environment, s.Installation)
	if err != nil || got.Channel != s.Channel {
		t.Fatal("canonical report rejected", err)
	}
	for _, bad := range [][]byte{append(bytes.Clone(body), '\n'), bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1), bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1)} {
		if _, err = decodeReport(bad, s.Environment, s.Installation); err == nil {
			t.Fatal("noncanonical or unsupported state accepted")
		}
	}
	if _, err = decodeReport(body, strings.Repeat("c", 64), s.Installation); err == nil {
		t.Fatal("foreign report accepted")
	}
}

func TestIssueReportRetainsAssessmentEvidenceWithoutResourceIdentity(t *testing.T) {
	s := reportWithIssue(t)
	s.Issues[0].Kind = "ASSESSMENT"
	s.Issues[0].Revision = strings.Repeat("c", 64)
	s.Issues[0].Code = "no_resource_ids"
	body, err := encodeReport(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeReport(body, s.Environment, s.Installation)
	if err != nil || got.Issues[0] != s.Issues[0] || got.Issues[0].ResourceIndex != -1 {
		t.Fatal("assessment without resources was lost", err)
	}
}

func TestIssueReportRejectsCapacityWithoutChangingPriorState(t *testing.T) {
	s := newReport(reportTestChannel, "installation-1", strings.Repeat("b", 64))
	s.Issues = make([]reportIssue, 1001)
	if _, err := encodeReport(s); err == nil {
		t.Fatal("issue overflow admitted")
	}
	s = newReport(reportTestChannel, "installation-1", strings.Repeat("b", 64))
	s.Priority = make([]string, 1001)
	if _, err := encodeReport(s); err == nil {
		t.Fatal("priority overflow admitted")
	}
	if _, err := decodeReport(bytes.Repeat([]byte{'x'}, (1<<20)+1), s.Environment, s.Installation); err == nil {
		t.Fatal("storage-sized report limit missing")
	}
}
