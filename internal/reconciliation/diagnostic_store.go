package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// DiagnosticObjects honors caller lifetimes and conditional writes without
// retries. Trusted bootstrap binds this backend and retained history to the
// environment. A storage result is never permission to dispatch a diagnostic.
type DiagnosticObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Replace(context.Context, string, string, []byte) (string, error)
	HistoryIsEmpty(context.Context, string) (bool, error)
}
type DiagnosticStore struct {
	objects                   DiagnosticObjects
	environment, installation string
}

// StoredDiagnostic binds the exact observed record/store/ETag. In particular,
// loading an existing SENDING record grants no new claim or dispatch authority.
type StoredDiagnostic struct {
	store  *DiagnosticStore
	record DiagnosticRecord
	etag   string
}

func NewDiagnosticStore(objects DiagnosticObjects, environment, installation string) (*DiagnosticStore, error) {
	s := &DiagnosticStore{objects, environment, installation}
	if !s.usable() {
		return nil, &DiagnosticStorageError{Code: "invalid_request"}
	}
	return s, nil
}
func (s *DiagnosticStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && diagnosticID.MatchString(s.installation)
}
func (s *DiagnosticStore) name(request string) string {
	return "control/diagnostics/" + s.installation + "/" + request + ".json"
}
func (s StoredDiagnostic) Record() DiagnosticRecord { return s.record }

var _ DiagnosticObjects = (*ocistorage.Client)(nil)
