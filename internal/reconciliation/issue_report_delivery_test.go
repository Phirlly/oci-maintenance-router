package reconciliation

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func reportWithIssue(t *testing.T) reportState {
	t.Helper()
	s := newReport(reportTestChannel, "installation-1", strings.Repeat("b", 64))
	id := noticeID(1)
	i := reportIssue{reportNoticeName(id), id, "SOURCE", "source_unavailable", "", "", -1}
	if err := replaceReportIssues(&s, []reportIssue{i}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIssueReportClaimsOnceAndCoalescesAcceptedSummary(t *testing.T) {
	s := reportWithIssue(t)
	if err := queueReport(&s); err != nil || s.Pending == nil {
		t.Fatal("summary not retained", err)
	}
	body := bytes.Clone(s.Pending.Payload)
	now := time.Now().UTC().Truncate(time.Millisecond)
	token := strings.Repeat("c", 64)
	if claimed, err := claimReport(&s, token, now, now.Add(time.Minute)); err != nil || !claimed {
		t.Fatal("first claim rejected", err)
	}
	if claimed, err := claimReport(&s, strings.Repeat("d", 64), now.Add(time.Second), now.Add(time.Minute)); err != nil || claimed || !bytes.Equal(body, s.Pending.Payload) {
		t.Fatal("cooldown or immutable bytes lost", err)
	}
	if err := finishReport(&s, token, 202, "message-1", now.Add(time.Second)); err != nil || s.Pending != nil || s.Accepted != s.Revision {
		t.Fatal("valid acceptance not retained", err)
	}
	if err := queueReport(&s); err != nil || s.Pending != nil {
		t.Fatal("accepted unchanged issues republished", err)
	}
}

func TestIssueReportUnknownOutcomeRetainsExactRetryAndRecurrence(t *testing.T) {
	s := reportWithIssue(t)
	_ = queueReport(&s)
	now := time.Now().UTC().Truncate(time.Millisecond)
	token := strings.Repeat("c", 64)
	_, _ = claimReport(&s, token, now, now.Add(time.Minute))
	body := bytes.Clone(s.Pending.Payload)
	if err := finishReport(&s, token, 202, "", now.Add(time.Second)); err != nil || s.Accepted != 0 || s.Pending == nil {
		t.Fatal("uncertain acceptance fabricated", err)
	}
	if claimed, err := claimReport(&s, strings.Repeat("d", 64), now.Add(5*time.Minute), now.Add(6*time.Minute)); err != nil || !claimed || !bytes.Equal(s.Pending.Payload, body) {
		t.Fatal("explicit operational retry changed the saved message", err)
	}
	if err := finishReport(&s, strings.Repeat("d", 64), 200, "message-2", now.Add(5*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	issues := append([]reportIssue{}, s.Issues...)
	_ = replaceReportIssues(&s, []reportIssue{})
	_ = replaceReportIssues(&s, issues)
	if err := queueReport(&s); err != nil || s.Pending == nil || bytes.Equal(s.Pending.Payload, body) {
		t.Fatal("recurrence suppressed after positive resolution", err)
	}
}

func TestIssueReportChannelReplacementDoesNotClaimOldAcceptance(t *testing.T) {
	s := reportWithIssue(t)
	_ = queueReport(&s)
	old := bytes.Clone(s.Pending.Payload)
	if err := bindReportChannel(&s, strings.Repeat("e", 64)); err != nil || s.Pending != nil || s.Accepted != 0 {
		t.Fatal("old recipient's pending message retained", err)
	}
	if err := queueReport(&s); err != nil || s.Pending == nil || bytes.Equal(s.Pending.Payload, old) {
		t.Fatal("new recipient inherited old suppression/message identity", err)
	}
}
