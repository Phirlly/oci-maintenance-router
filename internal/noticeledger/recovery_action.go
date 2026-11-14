package noticeledger

type RecoveryStatus string

const (
	RecoveryRejected RecoveryStatus = "REJECTED"
	RecoveryConsumed RecoveryStatus = "CONSUMED"
	RecoveryAccepted RecoveryStatus = "API_ACCEPTED"
	RecoveryReviewed RecoveryStatus = "REVIEW_REQUIRED"
	RecoveryApproved RecoveryStatus = "APPROVED"
	RecoveryClosed   RecoveryStatus = "CLOSED"
)

type RecoveryAttempt struct {
	BatchID      string `json:"batchId"`
	OwnerToken   string `json:"ownerToken"`
	AttemptCount uint64 `json:"attemptCount"`
	ExpiresAt    string `json:"expiresAt"`
}

// RecoveryAction is an owned view of one retained request and its result.
type RecoveryAction struct {
	Request            RecoveryRequest  `json:"request"`
	IntentDigest       string           `json:"intentDigest"`
	RecordedAt         string           `json:"recordedAt"`
	Status             RecoveryStatus   `json:"status"`
	Reason             string           `json:"reason"`
	CompletedAt        *string          `json:"completedAt"`
	Attempt            *RecoveryAttempt `json:"attempt"`
	BatchIDs           []string         `json:"batchIds"`
	InvalidatedAt      *string          `json:"invalidatedAt"`
	InvalidationReason string           `json:"invalidationReason"`
	Ambiguous          bool             `json:"ambiguous"`
}

type RecoverySnapshot struct{ Actions []RecoveryAction }
type recoveryData struct {
	Actions []RecoveryAction `json:"actions"`
}
