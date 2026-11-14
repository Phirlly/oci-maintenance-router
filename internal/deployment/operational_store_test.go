package deployment

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"oci-maintenance-router/internal/ocistorage"
)

// Synthetic object-service bytes and conditional versions; no coordinator state.
type operationalStore struct {
	mu      sync.Mutex
	pinned  *memoryStore
	objects map[string]ocistorage.Object
	history map[string]bool
	version int
	before  func(context.Context, string, string) error
}

func (s *operationalStore) GetVersion(ctx context.Context, name, version string) (ocistorage.Object, error) {
	return s.pinned.GetVersion(ctx, name, version)
}
func (s *operationalStore) begin(ctx context.Context, method, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.before != nil {
		return s.before(ctx, method, name)
	}
	return nil
}
func (s *operationalStore) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.begin(ctx, "GET", name); err != nil {
		return ocistorage.Object{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "configuration/router.json" {
		return ocistorage.Object{Body: bytes.Clone(s.pinned.config), ETag: "config"}, nil
	}
	if name == "configuration/catalog.json" {
		return ocistorage.Object{Body: bytes.Clone(s.pinned.catalog), ETag: "catalog"}, nil
	}
	obj, ok := s.objects[name]
	if !ok {
		return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
	}
	obj.Body = bytes.Clone(obj.Body)
	return obj, nil
}
func (s *operationalStore) Create(ctx context.Context, name string, body []byte) (string, error) {
	return s.write(ctx, "CREATE", name, "", body)
}
func (s *operationalStore) Replace(ctx context.Context, name, etag string, body []byte) (string, error) {
	return s.write(ctx, "REPLACE", name, etag, body)
}
func (s *operationalStore) write(ctx context.Context, method, name, etag string, body []byte) (string, error) {
	if err := s.begin(ctx, method, name); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.objects[name]
	if method == "CREATE" && ok || method == "REPLACE" && (!ok || old.ETag != etag) {
		return "", &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
	}
	s.version++
	tag := fmt.Sprintf("object-%d", s.version)
	s.objects[name] = ocistorage.Object{Body: bytes.Clone(body), ETag: tag}
	s.history[name] = true
	return tag, nil
}
func (s *operationalStore) ListNoticePage(ctx context.Context, _ string) (ocistorage.NoticePage, error) {
	if err := s.begin(ctx, "LIST", "notices/"); err != nil {
		return ocistorage.NoticePage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	names := []string{}
	for name := range s.objects {
		if strings.HasPrefix(name, "notices/") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return ocistorage.NoticePage{Names: names}, nil
}
func (s *operationalStore) HistoryIsEmpty(ctx context.Context, prefix string) (bool, error) {
	if err := s.begin(ctx, "HISTORY", prefix); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.history {
		if strings.HasPrefix(name, prefix) {
			return false, nil
		}
	}
	return true, nil
}
