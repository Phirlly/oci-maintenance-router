package reconciliation

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

type discoveryCall struct {
	method, name, tag string
	body              []byte
}

// A concurrency-safe conditional object service. Hooks run outside its mutex
// to simulate external changes or lost responses without fabricating handles.
type discoveryObjects struct {
	mu            sync.Mutex
	objects       map[string]ocistorage.Object
	version       int
	calls         []discoveryCall
	before, after func(context.Context, discoveryCall) error
	page          func(context.Context, string) (ocistorage.NoticePage, error)
}

func (s *discoveryObjects) seed(name string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version++
	s.objects[name] = ocistorage.Object{Body: slices.Clone(body), ETag: fmt.Sprint("v", s.version)}
}
func (s *discoveryObjects) begin(ctx context.Context, call discoveryCall) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	if s.before != nil {
		return s.before(ctx, call)
	}
	return nil
}
func (s *discoveryObjects) end(ctx context.Context, call discoveryCall) error {
	if s.after != nil {
		return s.after(ctx, call)
	}
	return ctx.Err()
}
func (s *discoveryObjects) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	call := discoveryCall{method: "GET", name: name}
	if err := s.begin(ctx, call); err != nil {
		return ocistorage.Object{}, err
	}
	s.mu.Lock()
	obj, ok := s.objects[name]
	obj.Body = slices.Clone(obj.Body)
	s.mu.Unlock()
	if !ok {
		return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
	}
	if err := s.end(ctx, call); err != nil {
		return ocistorage.Object{}, err
	}
	return obj, nil
}
func (s *discoveryObjects) Create(ctx context.Context, name string, body []byte) (string, error) {
	return s.write(ctx, discoveryCall{method: "CREATE", name: name, body: slices.Clone(body)})
}
func (s *discoveryObjects) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	return s.write(ctx, discoveryCall{method: "REPLACE", name: name, tag: tag, body: slices.Clone(body)})
}
func (s *discoveryObjects) write(ctx context.Context, call discoveryCall) (string, error) {
	if err := s.begin(ctx, call); err != nil {
		return "", err
	}
	s.mu.Lock()
	old, exists := s.objects[call.name]
	if (call.method == "CREATE" && exists) || (call.method == "REPLACE" && (!exists || old.ETag != call.tag)) {
		s.mu.Unlock()
		return "", &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
	}
	s.version++
	tag := fmt.Sprint("v", s.version)
	s.objects[call.name] = ocistorage.Object{Body: slices.Clone(call.body), ETag: tag}
	s.mu.Unlock()
	if err := s.end(ctx, call); err != nil {
		return "", err
	}
	return tag, nil
}
func (s *discoveryObjects) ListNoticePage(ctx context.Context, start string) (ocistorage.NoticePage, error) {
	call := discoveryCall{method: "LIST", name: start}
	if err := s.begin(ctx, call); err != nil {
		return ocistorage.NoticePage{}, err
	}
	if s.page != nil {
		return s.page(ctx, start)
	}
	s.mu.Lock()
	names := []string{}
	for name := range s.objects {
		if strings.HasPrefix(name, "notices/") {
			names = append(names, name)
		}
	}
	s.mu.Unlock()
	slices.Sort(names)
	if err := s.end(ctx, call); err != nil {
		return ocistorage.NoticePage{}, err
	}
	return ocistorage.NoticePage{Names: names}, nil
}
func (s *discoveryObjects) noticeWrites() []discoveryCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []discoveryCall
	for _, call := range s.calls {
		if (call.method == "CREATE" || call.method == "REPLACE") && strings.HasPrefix(call.name, "notices/") {
			call.body = slices.Clone(call.body)
			result = append(result, call)
		}
	}
	return result
}
func (s *discoveryObjects) record(t *testing.T, environment, id string) noticeledger.Record {
	t.Helper()
	s.mu.Lock()
	obj := s.objects[noticeName(id)]
	body := slices.Clone(obj.Body)
	s.mu.Unlock()
	r, err := noticeledger.Decode(body, environment, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
