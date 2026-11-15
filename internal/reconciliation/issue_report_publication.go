package reconciliation

import (
	"bytes"
	"encoding/json"
)

type reportMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func validReportPublication(s reportState) bool {
	if s.Accepted == 0 {
		if s.AcceptedID != "" || s.AcceptedAt != "" || s.AcceptedHash != "" || s.AcceptedStatus != 0 {
			return false
		}
	} else if _, ok := leaseTimestamp(s.AcceptedAt); !ok || !operatorOpaqueText(s.AcceptedID, 256) || !leaseDigest.MatchString(s.AcceptedHash) || (s.AcceptedStatus != 200 && s.AcceptedStatus != 202) {
		return false
	}
	p := s.Pending
	if p == nil {
		return true
	}
	if p.Revision == 0 || p.Revision > s.Revision || p.Revision <= s.Accepted || !leaseDigest.MatchString(p.Digest) || len(p.Payload) > 16<<10 {
		return false
	}
	var m reportMessage
	if json.Unmarshal(p.Payload, &m) != nil || m.Title != "OCI maintenance router: saved work needs review" || m.Body == "" {
		return false
	}
	canonical, _ := json.Marshal(m)
	if !bytes.Equal(canonical, p.Payload) || p.Digest != operatorDigest(string(p.Payload)) {
		return false
	}
	if p.Claim == "" {
		return p.IssuedAt == "" && p.Deadline == "" && p.Status == 0
	}
	i, okI := leaseTimestamp(p.IssuedAt)
	d, okD := leaseTimestamp(p.Deadline)
	n, okN := leaseTimestamp(s.NotBefore)
	return leaseDigest.MatchString(p.Claim) && okI && okD && okN && d.After(i) && n.After(i) && (p.Status == 0 || p.Status >= 100 && p.Status <= 599)
}
