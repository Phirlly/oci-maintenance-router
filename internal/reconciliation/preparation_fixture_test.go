package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

const preparationCatalog = `{"regions":{"us-ashburn-1":"iad","us-phoenix-1":"phx"},"mysqlServices":["Oracle HeatWave MySQL","MySQL"]}`

type preparationObjects struct {
	*discoveryObjects
	history func(context.Context, string) (bool, error)
}

func (s *preparationObjects) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	call := discoveryCall{method: "HISTORY", name: name}
	if err := s.begin(ctx, call); err != nil {
		return false, err
	}
	if s.history != nil {
		return s.history(ctx, name)
	}
	return true, s.end(ctx, call)
}

func preparationConfig(t *testing.T, changes ...func(map[string]any)) []byte {
	t.Helper()
	var c map[string]any
	if err := json.Unmarshal([]byte(candidateConfig), &c); err != nil {
		t.Fatal(err)
	}
	c["schemaVersion"], c["mode"], c["installationId"] = 2, "PREVIEW", "installation-1"
	c["preparation"] = map[string]any{"initialize": true}
	for _, change := range changes {
		change(c)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func preparationFor(t *testing.T, s *preparationObjects, cfg []byte, ns []notices.Notice) *BaselinePreparation {
	t.Helper()
	ids := []string{}
	byID := map[string]notices.Notice{}
	for _, n := range ns {
		ids = append(ids, n.Key().NoticeID)
		byID[n.Key().NoticeID] = n
	}
	a := announcementDetails{page: func(context.Context, string) (ociannouncements.Page, error) {
		return ociannouncements.Page{IDs: slices.Clone(ids)}, nil
	}, get: func(_ context.Context, id string) (notices.Notice, error) {
		if n, ok := byID[id]; ok {
			return n, nil
		}
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}}
	s.seed("configuration/router.json", cfg)
	s.seed("configuration/catalog.json", []byte(preparationCatalog))
	p, err := NewBaselinePreparation(cfg, []byte(preparationCatalog), candidateLocation(), a, s)
	if err != nil {
		t.Fatal(err)
	}
	p.discovery.wait = func(c context.Context, _ time.Duration) error { return c.Err() }
	p.discovery.now = baselineTime
	return p
}

func preparationFixture(t *testing.T, ns []notices.Notice, changes ...func(map[string]any)) (*BaselinePreparation, *preparationObjects, *Lease) {
	t.Helper()
	s := &preparationObjects{discoveryObjects: &discoveryObjects{objects: map[string]ocistorage.Object{}}}
	cfg := preparationConfig(t, changes...)
	p := preparationFor(t, s, cfg, ns)
	catalog, _ := configuration.DecodeCatalog([]byte(preparationCatalog))
	c, err := configuration.Decode(cfg, catalog)
	if err != nil {
		t.Fatal(err)
	}
	env, err := c.EnvironmentID(candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewLeaseManager(s, env)
	if err != nil {
		t.Fatal(err)
	}
	return p, s, acquireLease(t, m, leaseBudget(t))
}

func preparationFailed(t *testing.T, b Baseline, err error, code string) *PreparationError {
	t.Helper()
	var detail *PreparationError
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
	if b.ID() != "" || b.EnvironmentID() != "" || b.Layers() != nil {
		t.Fatal("failure returned completed baseline")
	}
	return detail
}
func preparationHead(t *testing.T, s *preparationObjects, lease *Lease) PreparedHistory {
	t.Helper()
	obj, err := s.Get(leaseBudget(t), preparedHistoryName)
	if err != nil {
		t.Fatal(err)
	}
	h, err := DecodePreparedHistory(obj.Body, lease.manager.environmentID, "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func preparationCalls(s *preparationObjects, method, name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c.method == method && (name == "" || c.name == name) {
			n++
		}
	}
	return n
}
func prepareSuccessfully(t *testing.T, p *BaselinePreparation, l *Lease) Baseline {
	t.Helper()
	b, err := p.Prepare(leaseBudget(t), l)
	if err != nil || b.ID() == "" {
		t.Fatalf("preparation failed: %v", err)
	}
	return b
}
