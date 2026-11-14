package reconciliation

import "time"

// OperatorChannel is transient desired-channel input, not an ACTIVE-subscription
// check. The endpoint is hashed exactly and is not retained in challenge state.
type OperatorChannel struct {
	TopicID, SubscriptionID, Protocol, Endpoint string
}

type OperatorIntent struct {
	EnvironmentID  string `json:"environmentId"`
	InstallationID string `json:"installationId"`
	RequestID      string `json:"requestId"`
	TopicID        string `json:"topicId"`
	SubscriptionID string `json:"subscriptionId"`
	Protocol       string `json:"protocol"`
	EndpointDigest string `json:"endpointDigest"`
}

// A coordinator must generate a fresh operation token for each claim and verify
// real ownership before publication. These types validate structure only.
type OperatorClaim struct {
	OwnerToken, ClaimToken string
	Now, ExpiresAt         time.Time
}
type OperatorClaimRef struct{ OwnerToken, ClaimToken string }
type OperatorChallengeState string

const (
	OperatorPublishing OperatorChallengeState = "PUBLISHING"
	OperatorAccepted   OperatorChallengeState = "API_ACCEPTED"
	OperatorReview     OperatorChallengeState = "REVIEW_REQUIRED"
)

type OperatorChallengeSnapshot struct {
	Intent                                        OperatorIntent
	IntentDigest, PayloadDigest, ConfirmationCode string
	Payload                                       []byte
	OwnerToken, ClaimToken                        string
	StartedAt, ExpiresAt, UpdatedAt, AcceptedAt   time.Time
	State                                         OperatorChallengeState
	HTTPStatus                                    int
	MessageID, Reason                             string
	Ambiguous                                     bool
}
type OperatorChallengeError struct{ Code string }

func (e *OperatorChallengeError) Error() string { return "operator challenge: " + e.Code }

// OperatorChallenge is an immutable proposal or decoded record, not proof of
// persistence, publication permission, human receipt or activation readiness.
type OperatorChallenge struct{ data, parent *operatorChallengeData }

func NewOperatorChallenge(intent OperatorIntent, claim OperatorClaim) (OperatorChallenge, error) {
	if !validOperatorIntent(intent) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_intent"}
	}
	start, expiry := claim.Now.UTC().Truncate(time.Millisecond), claim.ExpiresAt.UTC().Truncate(time.Millisecond)
	if !leaseDigest.MatchString(claim.OwnerToken) || !leaseDigest.MatchString(claim.ClaimToken) || !leaseWallTime(start) || !leaseWallTime(expiry) || !expiry.After(start) || expiry.Sub(start) > leaseDuration {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_claim"}
	}
	digest := operatorIntentDigest(intent)
	payload := operatorChallengePayload(operatorReceiptCode(digest))
	stamp := start.Format(leaseTimeFormat)
	d := &operatorChallengeData{SchemaVersion: 1, Intent: intent, IntentDigest: digest, Payload: payload, PayloadDigest: operatorDigest(payload), OwnerToken: claim.OwnerToken, ClaimToken: claim.ClaimToken, StartedAt: stamp, ExpiresAt: expiry.Format(leaseTimeFormat), UpdatedAt: stamp, State: OperatorPublishing}
	return OperatorChallenge{data: d}, nil
}
func (r OperatorChallenge) Matches(intent OperatorIntent) bool {
	return r.data != nil && validOperatorIntent(intent) && r.data.Intent == intent
}
