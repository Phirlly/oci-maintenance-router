package reconciliation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIssueReportFullEnvelopeLeavesRoomForClaimAndOutcome(t *testing.T) {
	s := reportWithIssue(t)
	issues := []reportIssue{}
	for i := 0; i < 1000; i++ {
		id := noticeID(i) + strings.Repeat("a", 500)
		issues = append(issues, reportIssue{reportNoticeName(id), id, "SOURCE", "source_unavailable", "", "", -1})
		s.Priority = append(s.Priority, fmt.Sprintf("notices/%064x.json", i))
		s.Cursor.Names = append(s.Cursor.Names, fmt.Sprintf("notices/%064x.json", i))
	}
	if err := replaceReportIssues(&s, issues); err != nil {
		t.Fatal(err)
	}
	active, _ := json.Marshal(s.Issues)
	if len(active) < 700<<10 {
		t.Fatal("fixture does not approach active capacity", len(active))
	}
	if err := queueReport(&s); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	token := strings.Repeat("c", 64)
	if claimed, err := claimReport(&s, token, now, now.Add(time.Minute)); err != nil || !claimed {
		t.Fatal(err)
	}
	for _, accepted := range []bool{false, true} {
		if accepted {
			if err := finishReport(&s, token, 202, strings.Repeat("m", 256), now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		body, err := encodeReport(s)
		if err != nil || len(body) > 1<<20 {
			t.Fatal("maximum work state cannot retain publication progress", len(body), err)
		}
		if _, err = decodeReport(body, s.Environment, s.Installation); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := encodeReport(s)
	oversized := append([]reportIssue{}, s.Issues...)
	for i := range oversized {
		oversized[i].NoticeID += strings.Repeat("b", 300)
		oversized[i].ObjectName = reportNoticeName(oversized[i].NoticeID)
	}
	if err := replaceReportIssues(&s, oversized); err == nil {
		t.Fatal("oversized observations accepted")
	}
	after, _ := encodeReport(s)
	if !bytes.Equal(before, after) {
		t.Fatal("capacity failure destroyed confirmed state")
	}
}

func TestIssueReportMessageBoundsDetailsAndIncludesBatchContext(t *testing.T) {
	s := reportWithIssue(t)
	for i := 2; i < 12; i++ {
		id := noticeID(i) + strings.Repeat("a", 3900)
		s.Issues = append(s.Issues, reportIssue{reportNoticeName(id), id, "BATCH", "attempt_uncertain", strings.Repeat("b", 64), strings.Repeat("c", 64), -1})
	}
	issues := append([]reportIssue{}, s.Issues...)
	s.Issues = []reportIssue{}
	if err := replaceReportIssues(&s, issues); err != nil {
		t.Fatal(err)
	}
	if err := queueReport(&s); err != nil || len(s.Pending.Payload) > 16<<10 {
		t.Fatal("unbounded publication", err)
	}
	var message reportMessage
	if json.Unmarshal(s.Pending.Payload, &message) != nil || !strings.Contains(message.Body, "Batch: ") || !strings.Contains(message.Body, "of 11 known issues") {
		t.Fatal("bounded message lost actionable context")
	}
}
