package deployment

import "encoding/json"

// This is deliberately separate from the response contract: response identifiers,
// receipt proofs and saved review details must never become log fields.
type invocationLog struct {
	Event         string             `json:"event"`
	Operation     string             `json:"operation,omitempty"`
	Mode          string             `json:"mode,omitempty"`
	Status        string             `json:"status"`
	Code          string             `json:"code,omitempty"`
	CleanupCode   string             `json:"cleanupCode,omitempty"`
	ScanCompleted bool               `json:"scanCompleted"`
	Processing    *ProcessingOutcome `json:"processing,omitempty"`
	Reporting     *ReportingOutcome  `json:"reporting,omitempty"`
}

func (h *Handler) logResult(result response) {
	if h.logger == nil {
		return
	}
	line := invocationLog{Event: "router_invocation", Status: result.legacy.Status, Code: result.legacy.Code}
	if r := result.operational; r != nil {
		line.Operation, line.Mode, line.Status, line.Code = r.Operation, r.Mode, r.Status, r.Code
		line.CleanupCode, line.ScanCompleted, line.Processing, line.Reporting = r.CleanupCode, r.ScanCompleted, r.Processing, r.Reporting
	}
	body, _ := json.Marshal(line)
	h.logger.Print(string(body))
}
