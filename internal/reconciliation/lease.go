package reconciliation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

// LeaseObjects must provide bounded, validated objects/ETags and conditional
// writes. Bootstrap owns its association with the manager's environment.
type LeaseObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Replace(context.Context, string, string, []byte) (string, error)
}

// LeaseManager coordinates claims; it grants no source or delivery authority.
// Concurrent acquisition requires a concurrency-safe adapter, as the OCI client is.
type LeaseManager struct {
	objects       LeaseObjects
	environmentID string
	now           func() time.Time
	token         func() string
}

// Lease is an immutable invocation-scoped ownership handle. Its original caller
// lifetime is retained deliberately: another context cannot revive canceled work.
// Expiry cannot fence an already-issued external request.
type Lease struct {
	manager  *LeaseManager
	record   leaseRecord
	etag     string
	origin   context.Context
	deadline time.Time
}

func NewLeaseManager(objects LeaseObjects, environmentID string) (*LeaseManager, error) {
	if objects == nil || !leaseDigest.MatchString(environmentID) {
		return nil, &LeaseError{Code: "invalid_request"}
	}
	return &LeaseManager{objects: objects, environmentID: environmentID, now: time.Now, token: newLeaseToken}, nil
}
func newLeaseToken() string {
	var value [32]byte
	// Pinned Go crypto/rand.Read fills the buffer or terminates; it returns no error.
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
func (m *LeaseManager) usable() bool {
	return m != nil && m.objects != nil && m.now != nil && m.token != nil && leaseDigest.MatchString(m.environmentID)
}
func (l *Lease) usable() bool {
	return l != nil && l.manager.usable() && l.origin != nil && l.etag != "" && !l.deadline.IsZero()
}
