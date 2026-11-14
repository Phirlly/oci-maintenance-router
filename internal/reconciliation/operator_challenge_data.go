package reconciliation

type operatorChallengeData struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Intent        OperatorIntent         `json:"intent"`
	IntentDigest  string                 `json:"intentDigest"`
	Payload       string                 `json:"payload"`
	PayloadDigest string                 `json:"payloadDigest"`
	OwnerToken    string                 `json:"ownerToken"`
	ClaimToken    string                 `json:"claimToken"`
	StartedAt     string                 `json:"startedAt"`
	ExpiresAt     string                 `json:"expiresAt"`
	UpdatedAt     string                 `json:"updatedAt"`
	State         OperatorChallengeState `json:"state"`
	HTTPStatus    int                    `json:"httpStatus"`
	MessageID     string                 `json:"messageId"`
	Reason        string                 `json:"reason"`
	Ambiguous     bool                   `json:"ambiguous"`
	AcceptedAt    *string                `json:"acceptedAt"`
}

func (r OperatorChallenge) Snapshot() OperatorChallengeSnapshot {
	if r.data == nil {
		return OperatorChallengeSnapshot{}
	}
	d := r.data
	start, _ := leaseTimestamp(d.StartedAt)
	expiry, _ := leaseTimestamp(d.ExpiresAt)
	updated, _ := leaseTimestamp(d.UpdatedAt)
	s := OperatorChallengeSnapshot{Intent: d.Intent, IntentDigest: d.IntentDigest, PayloadDigest: d.PayloadDigest, ConfirmationCode: operatorReceiptCode(d.IntentDigest), Payload: []byte(d.Payload), OwnerToken: d.OwnerToken, ClaimToken: d.ClaimToken, StartedAt: start, ExpiresAt: expiry, UpdatedAt: updated, State: d.State, HTTPStatus: d.HTTPStatus, MessageID: d.MessageID, Reason: d.Reason, Ambiguous: d.Ambiguous}
	if d.AcceptedAt != nil {
		s.AcceptedAt, _ = leaseTimestamp(*d.AcceptedAt)
	}
	return s
}
