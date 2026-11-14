package reconciliation

type diagnosticData struct {
	SchemaVersion int              `json:"schemaVersion"`
	Intent        DiagnosticIntent `json:"intent"`
	IntentDigest  string           `json:"intentDigest"`
	DedupKey      string           `json:"dedupKey"`
	Payload       string           `json:"payloadWithoutRoutingKey"`
	PayloadDigest string           `json:"payloadDigest"`
	SecretVersion int64            `json:"secretVersion"`
	OwnerToken    string           `json:"ownerToken"`
	ClaimToken    string           `json:"claimToken"`
	StartedAt     string           `json:"startedAt"`
	ExpiresAt     string           `json:"expiresAt"`
	UpdatedAt     string           `json:"updatedAt"`
	State         DiagnosticState  `json:"state"`
	HTTPStatus    int              `json:"httpStatus"`
	Reason        string           `json:"reason"`
	Ambiguous     bool             `json:"ambiguous"`
	AcceptedAt    *string          `json:"acceptedAt"`
}

func (r DiagnosticRecord) Snapshot() DiagnosticSnapshot {
	if r.data == nil {
		return DiagnosticSnapshot{}
	}
	d := r.data
	start, _ := leaseTimestamp(d.StartedAt)
	expiry, _ := leaseTimestamp(d.ExpiresAt)
	updated, _ := leaseTimestamp(d.UpdatedAt)
	s := DiagnosticSnapshot{Intent: d.Intent, IntentDigest: d.IntentDigest, DedupKey: d.DedupKey, PayloadDigest: d.PayloadDigest, ConfirmationCode: diagnosticReceiptCode(d.IntentDigest), PayloadWithoutRoutingKey: []byte(d.Payload), SecretVersion: d.SecretVersion, OwnerToken: d.OwnerToken, ClaimToken: d.ClaimToken, StartedAt: start, ExpiresAt: expiry, UpdatedAt: updated, State: d.State, HTTPStatus: d.HTTPStatus, Reason: d.Reason, Ambiguous: d.Ambiguous}
	if d.AcceptedAt != nil {
		s.AcceptedAt, _ = leaseTimestamp(*d.AcceptedAt)
	}
	return s
}
