package reconciliation

import "strings"

func (r *processingRun) reportCapacity(bytes int) error {
	if len(r.result.snapshot.Issues)+len(r.result.snapshot.Attempts) >= 1000 || bytes > 1<<20-r.textBytes {
		return &ProcessingError{Code: "result_capacity"}
	}
	return nil
}
func (r *processingRun) issue(issue ProcessingIssue) error {
	size := len(issue.NoticeID) + len(issue.RevisionDigest) + len(issue.ResourceID) + len(issue.BatchID) + len(issue.Code)
	if err := r.reportCapacity(size); err != nil {
		return err
	}
	issue.NoticeID, issue.RevisionDigest = strings.Clone(issue.NoticeID), strings.Clone(issue.RevisionDigest)
	issue.ResourceID, issue.BatchID, issue.Code = strings.Clone(issue.ResourceID), strings.Clone(issue.BatchID), strings.Clone(issue.Code)
	r.textBytes += size
	r.result.snapshot.Issues = append(r.result.snapshot.Issues, issue)
	return nil
}
