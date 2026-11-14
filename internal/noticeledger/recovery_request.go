package noticeledger

// ActionKind identifies an explicit operation on one exact saved target.
type ActionKind string

const (
	RetryBatch      ActionKind = "RETRY_BATCH"
	ApproveRevision ActionKind = "APPROVE_REVISION"
	CloseBatch      ActionKind = "CLOSE_BATCH"
	CloseRevision   ActionKind = "CLOSE_REVISION"
)

// RecoveryRequest binds operator intent, not current authorization or durability.
type RecoveryRequest struct {
	RequestID            string     `json:"requestId"`
	Kind                 ActionKind `json:"kind"`
	TargetID             string     `json:"targetId"`
	ReviewFingerprint    string     `json:"reviewFingerprint"`
	RoutingGeneration    string     `json:"routingGeneration"`
	ComponentFingerprint string     `json:"componentFingerprint"`
}
