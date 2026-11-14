package reconciliation

import "time"

// DiagnosticIntent is a desired test identity, not TEST/lease/send authority.
// SecretID is a reference only; key bytes are never part of this model.
type DiagnosticIntent struct {
	EnvironmentID   string `json:"environmentId"`
	InstallationID  string `json:"installationId"`
	RequestID       string `json:"requestId"`
	ReleaseSHA256   string `json:"releaseSha256"`
	CatalogSHA256   string `json:"catalogSha256"`
	PagerDutyRegion string `json:"pagerDutyRegion"`
	SecretID        string `json:"secretId"`
	NonProduction   bool   `json:"nonProduction"`
}

// The coordinator must generate a fresh ClaimToken for EACH operation, distinct
// from its lease identity. Validating these strings does not prove randomness.
type DiagnosticClaim struct {
	OwnerToken, ClaimToken string
	Now, ExpiresAt         time.Time
}
type DiagnosticClaimRef struct{ OwnerToken, ClaimToken string }
type DiagnosticState string

const (
	DiagnosticSending  DiagnosticState = "SENDING"
	DiagnosticAccepted DiagnosticState = "API_ACCEPTED"
	DiagnosticReview   DiagnosticState = "REVIEW_REQUIRED"
)

type DiagnosticSnapshot struct {
	Intent                                                  DiagnosticIntent
	IntentDigest, DedupKey, PayloadDigest, ConfirmationCode string
	PayloadWithoutRoutingKey                                []byte
	SecretVersion                                           int64
	OwnerToken, ClaimToken                                  string
	StartedAt, ExpiresAt, UpdatedAt, AcceptedAt             time.Time
	State                                                   DiagnosticState
	HTTPStatus                                              int
	Reason                                                  string
	Ambiguous                                               bool
}
type DiagnosticError struct{ Code string }

func (e *DiagnosticError) Error() string { return "diagnostic: " + e.Code }

// DiagnosticRecord is an immutable proposal/decoded record, never permission to
// dispatch. Loading SENDING cannot authorize a second send, even by the same owner.
type DiagnosticRecord struct{ data, parent *diagnosticData }

func NewDiagnostic(intent DiagnosticIntent, version int64, claim DiagnosticClaim) (DiagnosticRecord, error) {
	if !validDiagnosticIntent(intent) {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_intent"}
	}
	if version <= 0 {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_version"}
	}
	start, expiry := claim.Now.UTC().Truncate(time.Millisecond), claim.ExpiresAt.UTC().Truncate(time.Millisecond)
	if !leaseDigest.MatchString(claim.OwnerToken) || !leaseDigest.MatchString(claim.ClaimToken) || !leaseWallTime(start) || !leaseWallTime(expiry) || !expiry.After(start) || expiry.Sub(start) > leaseDuration {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_claim"}
	}
	digest, dedup := diagnosticIdentity(intent)
	payload := diagnosticPayload(dedup, diagnosticReceiptCode(digest))
	stamp := start.Format(leaseTimeFormat)
	d := &diagnosticData{SchemaVersion: 1, Intent: intent, IntentDigest: digest, DedupKey: dedup, Payload: payload, PayloadDigest: diagnosticDigest(payload), SecretVersion: version, OwnerToken: claim.OwnerToken, ClaimToken: claim.ClaimToken, StartedAt: stamp, ExpiresAt: expiry.Format(leaseTimeFormat), UpdatedAt: stamp, State: DiagnosticSending}
	return DiagnosticRecord{data: d}, nil
}
func (r DiagnosticRecord) Matches(intent DiagnosticIntent) bool {
	return r.data != nil && validDiagnosticIntent(intent) && r.data.Intent == intent
}
