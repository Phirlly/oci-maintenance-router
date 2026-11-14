package delivery

import (
	"time"
)

type State string

const (
	Prepared       State = "PREPARED"
	Sending        State = "SENDING"
	RetryWait      State = "RETRY_WAIT"
	APIAccepted    State = "API_ACCEPTED"
	ReviewRequired State = "REVIEW_REQUIRED"
	Closed         State = "CLOSED"
)

// AttemptKind describes the last attempt, never permission to dispatch.
type AttemptKind string

const (
	AutomaticAttempt AttemptKind = "AUTOMATIC"
	ManualAttempt    AttemptKind = "MANUAL"
)

type Reason string

const (
	ReasonTransient            Reason = "transient"
	ReasonNetwork              Reason = "network"
	ReasonExpired              Reason = "expired"
	ReasonPermanent            Reason = "permanent"
	ReasonHorizon              Reason = "retry_horizon"
	ReasonSourceChanged        Reason = "source_changed"
	ReasonSourceUnavailable    Reason = "source_unavailable"
	ReasonConfigurationChanged Reason = "configuration_changed"
	ReasonSecretUnavailable    Reason = "secret_unavailable"
	ReasonOperator             Reason = "operator"
)

// Preparation supplies a genuine draft and a verified Vault version number,
// never a key value. It does not prove that the key is still available.
type Preparation struct {
	Draft         Draft
	SecretVersion int64
}

// BatchSnapshot is an owned view, not trusted input for restoring a record.
type BatchSnapshot struct {
	Message                                         Snapshot
	SecretVersion                                   int64
	State                                           State
	AttemptCount                                    uint64
	AttemptKind                                     AttemptKind
	OwnerToken                                      string
	FirstAttemptAt, LastAttemptAt, AttemptExpiresAt time.Time
	UpdatedAt, NextAttemptAt, AcceptedAt            time.Time
	Reason                                          Reason
	Ambiguous                                       bool
}

// Record proposes immutable delivery state. Persist it conditionally together
// with the enclosing notice state; no operation here performs I/O or permits send.
type Record struct{ data *recordData }

type AttemptContext struct {
	OwnerToken, RoutingGeneration string
	Now, LeaseExpiresAt           time.Time
}

type AttemptRef struct {
	BatchID, OwnerToken string
	AttemptCount        uint64
}
