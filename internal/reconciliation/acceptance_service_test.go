package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

type acceptanceSend struct {
	destination string
	payload     []byte
	version     int64
	resources   []string
}

// This is synthetic external service state, never coordinator/readiness handles.
type acceptanceService struct {
	objects        *preparationObjects
	config         []byte
	history        map[string]bool
	notices        map[string]notices.Notice
	owners         map[string]string
	statuses       map[string]int
	blockedSecret  string
	currentVersion int64
	currentReads   []string
	pinnedReads    []string
	sends          []acceptanceSend
	previousOwner  string
}

func (s *acceptanceService) environment(t *testing.T) string {
	t.Helper()
	catalog, err := configuration.DecodeCatalog([]byte(preparationCatalog))
	if err != nil {
		t.Fatal(err)
	}
	c, err := configuration.Decode(s.config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	env, err := c.EnvironmentID(candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func (s *acceptanceService) reconstruct(t *testing.T) {
	t.Helper()
	old := s.objects
	next := &preparationObjects{discoveryObjects: &discoveryObjects{objects: map[string]ocistorage.Object{}}}
	retained := map[string]bool{}
	for name := range s.history {
		retained[name] = true
	}
	old.mu.Lock()
	next.version = old.version
	for name, obj := range old.objects {
		next.objects[name] = ocistorage.Object{Body: bytes.Clone(obj.Body), ETag: obj.ETag}
		retained[name] = true
	}
	for _, call := range old.calls {
		if call.method == "CREATE" || call.method == "REPLACE" {
			retained[call.name] = true
		}
	}
	old.mu.Unlock()
	// No delete operation is used here. Prior names nevertheless survive separately
	// so absence of a current object cannot erase retained-history service facts.
	next.history = func(ctx context.Context, name string) (bool, error) {
		for prior := range retained {
			if strings.HasPrefix(prior, name) {
				return false, nil
			}
		}
		next.mu.Lock()
		defer next.mu.Unlock()
		for current := range next.objects {
			if strings.HasPrefix(current, name) {
				return false, nil
			}
		}
		return true, ctx.Err()
	}
	s.objects, s.history, s.config = next, retained, bytes.Clone(s.config)
}

func (s *acceptanceService) source() announcementDetails {
	return announcementDetails{page: func(ctx context.Context, _ string) (ociannouncements.Page, error) {
		ids := []string{}
		for id := range s.notices {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		return ociannouncements.Page{IDs: ids}, ctx.Err()
	}, get: func(ctx context.Context, id string) (notices.Notice, error) {
		if n, ok := s.notices[id]; ok {
			return n, ctx.Err()
		}
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}}
}

func (s *acceptanceService) saved(t *testing.T, id string) noticeledger.Record {
	t.Helper()
	store, err := ocistorage.NewNoticeStore(s.objects, s.environment(t))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(leaseBudget(t), id)
	if err != nil {
		t.Fatal(err)
	}
	return saved.Record()
}

func (s *acceptanceService) review(t *testing.T, id string) noticeledger.SavedReview {
	t.Helper()
	r := s.saved(t, id)
	store, err := NewPreparationIssueStore(s.objects, s.environment(t))
	if err != nil {
		t.Fatal(err)
	}
	issues, err := store.Load(leaseBudget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.Review(issues.Record(), fmt.Sprintf("%x", sha256.Sum256(s.config)))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func acceptanceBatch(t *testing.T, r noticeledger.Record, destination string) delivery.BatchSnapshot {
	t.Helper()
	for _, b := range r.Snapshot().Batches {
		if b.Message.DestinationID == destination {
			return b
		}
	}
	t.Fatal("missing destination batch", destination)
	return delivery.BatchSnapshot{}
}

func acceptanceRelease(t *testing.T, lease *Lease) {
	t.Helper()
	cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := lease.Release(cleanup); err != nil {
		t.Fatal("release", err)
	}
}
