package deployment

import "encoding/json"

// This is deliberately separate from the response contract: response identifiers,
// receipt proofs and saved review details must never become log fields.
type invocationLog struct {
	Event          string                `json:"event"`
	Operation      string                `json:"operation,omitempty"`
	Mode           string                `json:"mode,omitempty"`
	Status         string                `json:"status"`
	Code           string                `json:"code,omitempty"`
	CleanupCode    string                `json:"cleanupCode,omitempty"`
	ScanCompleted  bool                  `json:"scanCompleted"`
	Processing     *ProcessingOutcome    `json:"processing,omitempty"`
	Reporting      *ReportingOutcome     `json:"reporting,omitempty"`
	Telemetry      *TelemetryOutcome     `json:"telemetry,omitempty"`
	InputRejection *inputRejection       `json:"inputRejection,omitempty"`
	PinnedRead     *pinnedReadDiagnostic `json:"pinnedRead,omitempty"`
	Worker         *workerDiagnostic     `json:"worker,omitempty"`
	WorkerCleanup  *workerDiagnostic     `json:"workerCleanup,omitempty"`
}

func (h *Handler) logResult(result response) {
	if h.logger == nil {
		return
	}
	line := invocationLog{Event: "router_invocation", Status: result.legacy.Status, Code: result.legacy.Code}
	line.InputRejection = result.inputRejection
	line.PinnedRead = result.legacy.pinnedRead
	if r := result.operational; r != nil {
		line.Operation, line.Mode, line.Status, line.Code = r.Operation, r.Mode, r.Status, r.Code
		line.CleanupCode, line.ScanCompleted, line.Processing, line.Reporting = r.CleanupCode, r.ScanCompleted, r.Processing, r.Reporting
		line.Telemetry = r.Telemetry
		line.Worker, line.WorkerCleanup = r.worker, r.workerCleanup
	}
	body, _ := json.Marshal(line)
	h.logger.Print(string(body))
}
