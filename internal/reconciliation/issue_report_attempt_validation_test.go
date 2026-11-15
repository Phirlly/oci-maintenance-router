package reconciliation

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
)

func TestIssueReportRejectsInvalidAttemptAndOutcomeWithoutMutation(t *testing.T) {
	for _, kind := range []string{"bad-token", "short-deadline", "reused", "stale", "past", "late", "bad-status"} {
		t.Run(kind, func(t *testing.T) {
			s := reportWithIssue(t)
			_ = queueReport(&s)
			now := time.Now().UTC().Truncate(time.Millisecond)
			token := strings.Repeat("c", 64)
			if kind != "bad-token" && kind != "short-deadline" {
				_, _ = claimReport(&s, token, now, now.Add(time.Minute))
			}
			before, _ := encodeReport(s)
			var err error
			switch kind {
			case "bad-token":
				_, err = claimReport(&s, "bad", now, now.Add(time.Minute))
			case "short-deadline":
				_, err = claimReport(&s, token, now, now.Add(time.Nanosecond))
			case "reused":
				_, err = claimReport(&s, token, now.Add(6*time.Minute), now.Add(7*time.Minute))
			case "stale":
				err = finishReport(&s, strings.Repeat("d", 64), 202, "message", now)
			case "past":
				err = finishReport(&s, token, 202, "message", now.Add(-time.Second))
			case "late":
				err = finishReport(&s, token, 202, "message", now.Add(time.Minute))
			case "bad-status":
				err = finishReport(&s, token, 600, "message", now)
			}
			after, _ := encodeReport(s)
			if err == nil || !bytes.Equal(before, after) {
				t.Fatal("invalid transition changed saved evidence", err)
			}
		})
	}
}

func TestIssueReportRevisionExhaustionPreservesHistory(t *testing.T) {
	s := reportWithIssue(t)
	s.Revision = math.MaxUint64
	before, _ := encodeReport(s)
	if replaceReportIssues(&s, []reportIssue{}) == nil || bindReportChannel(&s, strings.Repeat("e", 64)) == nil {
		t.Fatal("revision wrap admitted")
	}
	after, _ := encodeReport(s)
	if !bytes.Equal(before, after) {
		t.Fatal("revision exhaustion changed history")
	}
}
