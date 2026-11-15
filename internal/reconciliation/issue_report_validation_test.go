package reconciliation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIssueReportRejectsInvalidRetainedEvidence(t *testing.T) {
	for _, kind := range []string{"clock", "duplicate", "conflicting-reason", "zero-revision", "bad-object", "bad-notice", "kind", "names", "offset", "lower-bound", "next", "accepted-fields", "accepted-receipt", "pending-revision", "payload-shape", "payload-digest"} {
		t.Run(kind, func(t *testing.T) {
			s := reportWithIssue(t)
			switch kind {
			case "clock":
				s.NotBefore = "bad"
			case "duplicate":
				s.Issues = append(s.Issues, s.Issues[0])
			case "conflicting-reason":
				other := s.Issues[0]
				other.Code = "source_unknown"
				s.Issues = append(s.Issues, other)
			case "zero-revision":
				s.Revision = 0
			case "bad-object":
				s.Issues[0].ObjectName = "../private"
			case "bad-notice":
				s.Issues[0].NoticeID = noticeID(2)
			case "kind":
				s.Issues[0].Kind = "OTHER"
			case "names":
				s.Priority = []string{reportNoticeName(noticeID(1)), reportNoticeName(noticeID(1))}
			case "offset":
				s.Cursor.Offset = 1
			case "lower-bound":
				s.Cursor.Start = "notices/" + strings.Repeat("f", 64) + ".json"
				s.Cursor.Names = []string{reportNoticeName(noticeID(1))}
			case "next":
				s.Cursor.Next = "bad"
			case "accepted-fields":
				s.AcceptedID = "invented"
			case "accepted-receipt":
				s.Accepted = 1
			case "pending-revision":
				_ = queueReport(&s)
				s.Pending.Revision = 2
			case "payload-shape":
				_ = queueReport(&s)
				s.Pending.Payload = json.RawMessage(`{"title":"wrong","body":"private"}`)
			case "payload-digest":
				_ = queueReport(&s)
				s.Pending.Digest = strings.Repeat("a", 64)
			}
			if _, err := encodeReport(s); err == nil {
				t.Fatal("invalid evidence admitted")
			}
		})
	}
}
