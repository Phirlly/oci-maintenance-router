// Package delivery prepares credential-free drafts for newly selected work.
// Drafts are not persisted batches, receipts or permission to send. Existing
// assignments and retries must retain their first persisted bytes and key version.
package delivery

import (
	"slices"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// Snapshot is a caller-owned view, not a persistence or HTTP request schema.
// SecretID is a Vault reference, never a credential. No key version is invented.
type Snapshot struct {
	BatchID                  string
	EnvironmentID            string
	Revision                 notices.RevisionKey
	RoutingGeneration        string
	DestinationID            string
	SecretID                 string
	PagerDutyRegion          string
	Resources                []routing.Target
	DedupKey                 string
	PayloadWithoutRoutingKey []byte
	PayloadDigest            string
}

type Draft struct{ snapshot Snapshot }

func (d Draft) Snapshot() Snapshot {
	s := d.snapshot
	s.Resources = slices.Clone(s.Resources)
	s.PayloadWithoutRoutingKey = slices.Clone(s.PayloadWithoutRoutingKey)
	return s
}

type CapacityIssue struct {
	ResourceID string
	Code       string
}

type Result struct {
	Drafts []Draft
	Issues []CapacityIssue
}

// Error never includes source values or untrusted field names.
type Error struct {
	Field string
	Code  string
}

func (e *Error) Error() string { return e.Field + ": " + e.Code }
