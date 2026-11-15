package reconciliation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func reportNoticeName(id string) string {
	return fmt.Sprintf("notices/%x.json", sha256.Sum256([]byte(id)))
}

func reportIssueSubject(i reportIssue) string {
	storage := ""
	if i.Kind == "STORE" {
		storage = i.Code
	}
	body, _ := json.Marshal([]any{i.ObjectName, i.Kind, i.Revision, i.ResourceIndex, i.BatchID, storage})
	return string(body)
}
func reportIssueKey(i reportIssue) string { return reportIssueSubject(i) + ":" + i.Code }

func validReportIssue(i reportIssue) bool {
	if !reportObjectPattern.MatchString(i.ObjectName) || !reportCodePattern.MatchString(i.Code) {
		return false
	}
	if i.Kind == "STORE" {
		return i.NoticeID == "" && i.Revision == "" && i.BatchID == "" && i.ResourceIndex == -1 &&
			(i.Code == "notice_unreadable" || i.Code == "preparation_unreadable")
	}
	if len(i.NoticeID) > 4096 || !reportNoticePattern.MatchString(i.NoticeID) || reportNoticeName(i.NoticeID) != i.ObjectName {
		return false
	}
	switch i.Kind {
	case "RESOURCE", "PREPARATION":
		return leaseDigest.MatchString(i.Revision) && i.BatchID == "" && i.ResourceIndex >= 0 && i.ResourceIndex < 1000000 && i.Code != "lookup_missing"
	case "ASSESSMENT":
		return leaseDigest.MatchString(i.Revision) && i.BatchID == "" && i.ResourceIndex == -1
	case "BATCH":
		return leaseDigest.MatchString(i.Revision) && leaseDigest.MatchString(i.BatchID) && i.ResourceIndex == -1
	case "NOTICE", "SOURCE":
		return i.Revision == "" && i.BatchID == "" && i.ResourceIndex == -1
	}
	return false
}
