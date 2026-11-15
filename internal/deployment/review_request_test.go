package deployment

import (
	"strings"
	"testing"
)

func TestReviewCommandAcceptsExistingAnnouncementIDLimit(t *testing.T) {
	id := "ocid1.announcement.oc1.." + strings.Repeat("a", 4096-len("ocid1.announcement.oc1.."))
	for _, body := range []string{reviewInvocation(id, ""), reviewInvocation(id, strings.Repeat("a", 64)+":1")} {
		if _, err := parseCommand([]byte(body)); err != nil {
			t.Fatalf("valid bounded review command rejected: %v", err)
		}
	}
}

func TestReviewCommandRejectsAuthorityAndMalformedFields(t *testing.T) {
	id := "ocid1.announcement.oc1..example"
	for _, body := range []string{
		`{"operation":"review-notice"}`,
		`{"operation":"review-notice","noticeId":null}`,
		`{"operation":"review-notice","noticeId":"` + id + `","noticeId":"` + id + `"}`,
		`{"operation":"review-notice","noticeId":"` + id + `","mode":"LIVE"}`,
		`{"operation":"reconcile","noticeId":"` + id + `"}`,
		reviewInvocation(id+"/private", ""),
		reviewInvocation(id+strings.Repeat("a", 4096), ""),
		reviewInvocation(id, "bad"),
		reviewInvocation(id, "") + `{}`,
		strings.Repeat(" ", 8192) + reviewInvocation(id, ""),
		strings.Repeat(" ", 1024) + `{"operation":"reconcile"}`,
	} {
		if _, err := parseCommand([]byte(body)); err == nil {
			t.Fatalf("invalid review request accepted: %q", body)
		}
	}
}

func TestReviewCommandPreservesOperationSpecificByteBudgets(t *testing.T) {
	for _, tc := range []struct {
		body  string
		limit int
	}{
		{reviewInvocation("ocid1.announcement.oc1..example", ""), 8192},
		{`{"operation":"reconcile"}`, 1024},
	} {
		padded := tc.body + strings.Repeat(" ", tc.limit-len(tc.body))
		if _, err := parseCommand([]byte(padded)); err != nil {
			t.Fatal("exact request budget rejected", err)
		}
		if _, err := parseCommand([]byte(padded + " ")); err == nil {
			t.Fatal("oversized request accepted")
		}
	}
}
