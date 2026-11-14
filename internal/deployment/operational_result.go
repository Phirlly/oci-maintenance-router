package deployment

// OperationalResult reports bounded confirmed work, never receipt codes or state.
// Version 2 is separate from the legacy nonsending installation-check contract.
type OperationalResult struct {
	ResponseVersion     int                `json:"responseVersion"`
	Status              string             `json:"status"`
	Operation           string             `json:"operation"`
	Mode                string             `json:"mode"`
	CheckAttemptID      string             `json:"checkAttemptId,omitempty"`
	ConfigurationSHA256 string             `json:"configurationSha256"`
	ReleaseSHA256       string             `json:"releaseSha256"`
	RoutingGeneration   string             `json:"routingGeneration"`
	Code                string             `json:"code,omitempty"`
	CleanupCode         string             `json:"cleanupCode,omitempty"`
	BaselinePrepared    bool               `json:"baselinePrepared"`
	BaselineSelected    bool               `json:"baselineSelected"`
	ScanCompleted       bool               `json:"scanCompleted"`
	Diagnostic          *TestOutcome       `json:"diagnostic,omitempty"`
	Operator            *TestOutcome       `json:"operator,omitempty"`
	Processing          *ProcessingOutcome `json:"processing,omitempty"`
	Recovery            *RecoveryOutcome   `json:"recovery,omitempty"`
}

type TestOutcome struct {
	State          string `json:"state"`
	Dispatched     bool   `json:"dispatched"`
	ObservedStatus int    `json:"observedStatus,omitempty"`
	Code           string `json:"code,omitempty"`
}

type ProcessingOutcome struct {
	Prepared   int `json:"prepared"`
	Accepted   int `json:"accepted"`
	Deferred   int `json:"deferred"`
	Suppressed int `json:"suppressed"`
	Issues     int `json:"issues"`
	Attempts   int `json:"attempts"`
}

type RecoveryOutcome struct {
	RequestID          string `json:"requestId"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	InvalidationReason string `json:"invalidationReason,omitempty"`
	Ambiguous          bool   `json:"ambiguous"`
}

type response struct {
	legacy      Result
	operational *OperationalResult
}

func rejected(code string) response { return response{legacy: failed(code)} }

func (r response) output() (any, bool) {
	if r.operational == nil {
		return r.legacy, r.legacy.Status == "deployment-verified"
	}
	switch r.operational.Status {
	case "operational-verified", "reconciled", "prepared", "idle", "recovery-recorded":
		return r.operational, true
	default:
		return r.operational, false
	}
}
