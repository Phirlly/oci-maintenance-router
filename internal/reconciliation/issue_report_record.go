package reconciliation

import "encoding/json"

const issueReportName = "control/operator-reporting.json"

// Reporting observations and publication progress never grant business-delivery
// authority. The notice and preparation records remain the source of truth.
type reportIssue struct {
	ObjectName    string `json:"objectName"`
	NoticeID      string `json:"noticeId"`
	Kind          string `json:"kind"`
	Code          string `json:"code"`
	Revision      string `json:"revision"`
	BatchID       string `json:"batchId"`
	ResourceIndex int    `json:"resourceIndex"`
}

type reportCursor struct {
	Start  string   `json:"start"`
	Names  []string `json:"names"`
	Offset int      `json:"offset"`
	Next   string   `json:"next"`
}

type reportPublication struct {
	Revision uint64          `json:"revision"`
	Digest   string          `json:"digest"`
	Payload  json.RawMessage `json:"payload"`
	Claim    string          `json:"claim"`
	IssuedAt string          `json:"issuedAt"`
	Deadline string          `json:"deadline"`
	Status   int             `json:"status"`
}

type reportState struct {
	SchemaVersion  int                `json:"schemaVersion"`
	Environment    string             `json:"environment"`
	Installation   string             `json:"installation"`
	Channel        string             `json:"channel"`
	Revision       uint64             `json:"revision"`
	Accepted       uint64             `json:"accepted"`
	AcceptedID     string             `json:"acceptedId"`
	AcceptedAt     string             `json:"acceptedAt"`
	AcceptedHash   string             `json:"acceptedHash"`
	AcceptedStatus int                `json:"acceptedStatus"`
	Issues         []reportIssue      `json:"issues"`
	Priority       []string           `json:"priority"`
	Cursor         reportCursor       `json:"cursor"`
	NotBefore      string             `json:"notBefore"`
	Pending        *reportPublication `json:"pending,omitempty"`
}

func newReport(environment, installation, channel string) reportState {
	return reportState{SchemaVersion: 1, Environment: environment, Installation: installation,
		Channel: channel, Issues: []reportIssue{}, Priority: []string{}, Cursor: reportCursor{Names: []string{}}}
}
