package deployment

// OperationalResult reports bounded work and whitelisted saved-review evidence,
// never receipt codes, credentials or complete business records.
// Version 2 is separate from the legacy nonsending installation-check contract.
type OperationalResult struct {
	ResponseVersion     int                 `json:"responseVersion"`
	Status              string              `json:"status"`
	Operation           string              `json:"operation"`
	Mode                string              `json:"mode"`
	CheckAttemptID      string              `json:"checkAttemptId,omitempty"`
	ConfigurationSHA256 string              `json:"configurationSha256"`
	ReleaseSHA256       string              `json:"releaseSha256"`
	RoutingGeneration   string              `json:"routingGeneration"`
	Code                string              `json:"code,omitempty"`
	CleanupCode         string              `json:"cleanupCode,omitempty"`
	BaselinePrepared    bool                `json:"baselinePrepared"`
	BaselineSelected    bool                `json:"baselineSelected"`
	ScanCompleted       bool                `json:"scanCompleted"`
	Diagnostic          *TestOutcome        `json:"diagnostic,omitempty"`
	Operator            *TestOutcome        `json:"operator,omitempty"`
	Processing          *ProcessingOutcome  `json:"processing,omitempty"`
	Reporting           *ReportingOutcome   `json:"reporting,omitempty"`
	Telemetry           *TelemetryOutcome   `json:"telemetry,omitempty"`
	Recovery            *RecoveryOutcome    `json:"recovery,omitempty"`
	Review              *SavedReviewPage    `json:"review,omitempty"`
	SavedNotices        *SavedNoticePage    `json:"savedNotices,omitempty"`
	ClosureReview       *ClosureReviewPage  `json:"closureReview,omitempty"`
	RecoveryReview      *RecoveryReviewPage `json:"recoveryReview,omitempty"`
}

type TestOutcome struct {
	State           string `json:"state"`
	Dispatched      bool   `json:"dispatched"`
	ObservedStatus  int    `json:"observedStatus,omitempty"`
	Code            string `json:"code,omitempty"`
	ReceiptVerified bool   `json:"receiptVerified,omitempty"`
}

type ProcessingOutcome struct {
	Prepared   int `json:"prepared"`
	Accepted   int `json:"accepted"`
	Deferred   int `json:"deferred"`
	Suppressed int `json:"suppressed"`
	Issues     int `json:"issues"`
	Attempts   int `json:"attempts"`
}

type ReportingOutcome struct {
	Known      bool   `json:"known"`
	Issues     int    `json:"issues"`
	Reviewed   int    `json:"reviewed"`
	Remaining  int    `json:"remaining"`
	Deferred   int    `json:"deferred"`
	Pending    bool   `json:"pending"`
	Dispatched bool   `json:"dispatched"`
	Accepted   bool   `json:"accepted"`
	Code       string `json:"code,omitempty"`
}

type TelemetryOutcome struct {
	Accepted bool   `json:"accepted"`
	Code     string `json:"code,omitempty"`
}

type RecoveryOutcome struct {
	RequestID          string `json:"requestId"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	InvalidationReason string `json:"invalidationReason,omitempty"`
	Ambiguous          bool   `json:"ambiguous"`
}

type response struct {
	legacy         Result
	operational    *OperationalResult
	inputRejection *inputRejection // Logging only; output never serializes this wrapper.
}

func rejected(code string) response { return response{legacy: failed(code)} }

func (r response) output() (any, bool) {
	if r.operational == nil {
		return r.legacy, r.legacy.Status == "deployment-verified"
	}
	switch r.operational.Status {
	case "coalesced":
		// A duplicate wake-up is handled by the current owner, not failed work.
		// Apply and explicit operator actions still require their own result.
		return r.operational, r.operational.Operation == "reconcile"
	case "operational-verified", "reconciled", "prepared", "idle", "recovery-recorded", "reviewed", "listed", "closure-completed", "recovery-verified":
		return r.operational, true
	default:
		return r.operational, false
	}
}
