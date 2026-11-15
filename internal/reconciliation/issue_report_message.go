package reconciliation

import (
	"encoding/json"
	"fmt"
	"math"
)

func queueReport(s *reportState) error {
	if s.Pending != nil || len(s.Issues) == 0 || s.Revision == s.Accepted {
		return nil
	}
	identity, _ := json.Marshal([]any{"operator-report-v1", s.Environment, s.Installation, s.Channel, s.Revision, s.Issues})
	key := operatorDigest(string(identity))
	body := fmt.Sprintf("OCI maintenance router has %d saved issue(s) needing review.\nThese are last saved observations, not current dependency-health checks.\nReport: %s\n", len(s.Issues), key)
	shown := 0
	for _, issue := range s.Issues {
		reference := issue.NoticeID
		if reference == "" {
			reference = issue.ObjectName
		}
		line := fmt.Sprintf("\n%s: %s\nReference: %s\n", issue.Kind, issue.Code, reference)
		if issue.Revision != "" {
			line += "Revision: " + issue.Revision + "\n"
		}
		if issue.ResourceIndex >= 0 {
			line += fmt.Sprintf("Resource index: %d\n", issue.ResourceIndex)
		}
		if issue.BatchID != "" {
			line += "Batch: " + issue.BatchID + "\n"
		}
		if shown == 10 || len(body)+len(line) > 12<<10 {
			break
		}
		body += line
		shown++
	}
	body += fmt.Sprintf("\nShowing %d of %d known issues. Use the router's read-only list-notices and review-notice operations for retained details. Do not replay accepted work without reviewing its history.\n", shown, len(s.Issues))
	payload, _ := json.Marshal(reportMessage{"OCI maintenance router: saved work needs review", body})
	candidate := *s
	candidate.Pending = &reportPublication{Revision: s.Revision, Digest: operatorDigest(string(payload)), Payload: payload}
	if _, err := encodeReport(candidate); err != nil {
		return err
	}
	*s = candidate
	return nil
}

func bindReportChannel(s *reportState, channel string) error {
	if s.Channel == channel {
		return nil
	}
	if !leaseDigest.MatchString(channel) || s.Revision == math.MaxUint64 {
		return &ReportingError{"invalid_channel"}
	}
	candidate := *s
	candidate.Channel, candidate.Revision, candidate.Accepted = channel, s.Revision+1, 0
	candidate.Pending = nil
	candidate.AcceptedID, candidate.AcceptedAt, candidate.AcceptedHash, candidate.AcceptedStatus = "", "", "", 0
	if _, err := encodeReport(candidate); err != nil {
		return err
	}
	*s = candidate
	return nil
}
