package reconciliation

import (
	"encoding/json"
	"math"
	"slices"
	"strings"

	"oci-maintenance-router/internal/noticeledger"
)

func mergeReportNotice(s *reportState, r noticeledger.Record, p noticeledger.PreparationIssues, preparationKnown bool, config string) error {
	env, id := r.Identity()
	if env != s.Environment || id == "" {
		return &ReportingError{"notice_invalid"}
	}
	view, err := r.Review(p, config)
	if err != nil {
		return &ReportingError{"notice_invalid"}
	}
	name := reportNoticeName(id)
	observed := map[string]reportIssue{}
	for _, i := range view.Issues {
		if i.Kind == "RESOURCE" && i.Code == "lookup_missing" {
			continue
		}
		v := reportIssue{name, id, i.Kind, i.Code, i.RevisionDigest, i.BatchID, i.ResourceIndex}
		observed[reportIssueSubject(v)] = v
	}
	for _, i := range view.Preparation {
		if i.Status == "LAST_BLOCKED" {
			v := reportIssue{name, id, "PREPARATION", i.Observation.Code, i.Observation.RevisionDigest, "", i.Observation.ResourceIndex}
			observed[reportIssueSubject(v)] = v
		}
	}
	next := make([]reportIssue, 0, len(s.Issues)+len(observed))
	for _, old := range s.Issues {
		if old.ObjectName != name {
			next = append(next, old)
			continue
		}
		if _, replaced := observed[reportIssueSubject(old)]; replaced || reportIssueResolved(old, r.Snapshot(), view, preparationKnown) {
			continue
		}
		next = append(next, old)
	}
	for _, issue := range observed {
		next = append(next, issue)
	}
	return replaceReportIssues(s, next)
}

func replaceReportIssues(s *reportState, next []reportIssue) error {
	slices.SortFunc(next, func(a, b reportIssue) int { return strings.Compare(reportIssueKey(a), reportIssueKey(b)) })
	before, _ := json.Marshal(s.Issues)
	after, _ := json.Marshal(next)
	if string(before) == string(after) {
		return nil
	}
	if s.Revision == math.MaxUint64 {
		return &ReportingError{"capacity"}
	}
	candidate := *s
	candidate.Issues, candidate.Revision = next, s.Revision+1
	if _, err := encodeReport(candidate); err != nil {
		return err
	}
	*s = candidate
	return nil
}
