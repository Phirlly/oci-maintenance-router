package reconciliation

import "slices"

// ProcessingIssue contains controlled identifiers/reasons, never source text,
// owner tags, credentials or secret references. It is not a published alert.
type ProcessingIssue struct{ NoticeID, RevisionDigest, ResourceID, BatchID, Code string }

// ProcessingAttempt separates a called transport and observed response from
// confirmed outcome persistence. Neither HTTP202 nor Confirmed proves receipt.
type ProcessingAttempt struct {
	NoticeID, BatchID      string
	Status                 int
	MayHaveSent, Confirmed bool
}

type ProcessingSnapshot struct {
	Completed                                bool
	Prepared, Accepted, Deferred, Suppressed int
	Issues                                   []ProcessingIssue
	Attempts                                 []ProcessingAttempt
}

type ProcessingResult struct{ snapshot ProcessingSnapshot }

func (r ProcessingResult) Snapshot() ProcessingSnapshot {
	s := r.snapshot
	s.Issues, s.Attempts = slices.Clone(s.Issues), slices.Clone(s.Attempts)
	return s
}
