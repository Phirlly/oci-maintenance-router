package reconciliation

import (
	"bytes"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

var reportObjectPattern = regexp.MustCompile(`^notices/[0-9a-f]{64}\.json$`)
var reportNoticePattern = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var reportCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type ReportingError struct{ Code string }

func (e *ReportingError) Error() string { return "operator reporting: " + e.Code }

func encodeReport(s reportState) ([]byte, error) {
	if !validReport(s) {
		return nil, &ReportingError{"invalid_record"}
	}
	body, _ := json.Marshal(s)
	issues, _ := json.Marshal(s.Issues)
	if len(body) > 1<<20 || len(issues) > 768<<10 {
		return nil, &ReportingError{"capacity"}
	}
	return body, nil
}

func decodeReport(body []byte, environment, installation string) (reportState, error) {
	var s reportState
	if len(body) > 1<<20 || !utf8.Valid(body) || json.Unmarshal(body, &s) != nil {
		return s, &ReportingError{"invalid_record"}
	}
	canonical, err := encodeReport(s)
	if err != nil || !bytes.Equal(body, canonical) || s.Environment != environment || s.Installation != installation {
		return reportState{}, &ReportingError{"invalid_record"}
	}
	return s, nil
}

func validReport(s reportState) bool {
	if s.SchemaVersion != 1 || !leaseDigest.MatchString(s.Environment) || !operatorLocalID.MatchString(s.Installation) ||
		!leaseDigest.MatchString(s.Channel) || s.Accepted > s.Revision || s.Issues == nil || len(s.Issues) > 1000 || !validReportNames(s.Priority, false) || !validReportCursor(s.Cursor) {
		return false
	}
	if s.NotBefore != "" {
		if _, ok := leaseTimestamp(s.NotBefore); !ok {
			return false
		}
	}
	previous := ""
	subjects := map[string]bool{}
	for _, issue := range s.Issues {
		key := reportIssueKey(issue)
		subject := reportIssueSubject(issue)
		if !validReportIssue(issue) || key <= previous || subjects[subject] {
			return false
		}
		subjects[subject] = true
		previous = key
	}
	if len(s.Issues) != 0 && s.Revision == 0 {
		return false
	}
	return validReportPublication(s)
}

func validReportNames(names []string, sorted bool) bool {
	if names == nil || len(names) > 1000 {
		return false
	}
	seen := map[string]bool{}
	previous := ""
	for _, name := range names {
		if !reportObjectPattern.MatchString(name) || seen[name] || sorted && name <= previous {
			return false
		}
		seen[name], previous = true, name
	}
	return true
}

func validReportCursor(c reportCursor) bool {
	if !validReportNames(c.Names, true) || c.Offset < 0 || c.Offset > len(c.Names) || c.Start != "" && !reportObjectPattern.MatchString(c.Start) {
		return false
	}
	if c.Next != "" && (!reportObjectPattern.MatchString(c.Next) || c.Next <= c.Start) {
		return false
	}
	for _, name := range c.Names {
		if name < c.Start {
			return false
		}
	}
	return true
}
