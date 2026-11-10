package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

const leaseName = "control/lease.json"

var _ LeaseObjects = (*ocistorage.Client)(nil)

type leaseCall struct {
	method, name, etag string
	body               []byte
}
type leaseMemory struct {
	mu      sync.Mutex
	body    []byte
	version int
	calls   []leaseCall
}

func (s *leaseMemory) Get(_ context.Context, name string) (ocistorage.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, leaseCall{method: "GET", name: name})
	if s.version == 0 {
		return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 404}
	}
	return ocistorage.Object{Body: slices.Clone(s.body), ETag: fmt.Sprint("v", s.version)}, nil
}
func (s *leaseMemory) Create(_ context.Context, name string, body []byte) (string, error) {
	return s.write("CREATE", name, "", body)
}
func (s *leaseMemory) Replace(_ context.Context, name, tag string, body []byte) (string, error) {
	return s.write("REPLACE", name, tag, body)
}
func (s *leaseMemory) write(method, name, tag string, body []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, leaseCall{method: method, name: name, etag: tag, body: slices.Clone(body)})
	if (method == "CREATE" && s.version != 0) || (method == "REPLACE" && (s.version == 0 || tag != fmt.Sprint("v", s.version))) {
		return "", &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
	}
	s.version++
	s.body = slices.Clone(body)
	return fmt.Sprint("v", s.version), nil
}

type leaseFunctions struct {
	get     func(context.Context, string) (ocistorage.Object, error)
	create  func(context.Context, string, []byte) (string, error)
	replace func(context.Context, string, string, []byte) (string, error)
}

func (s leaseFunctions) Get(c context.Context, n string) (ocistorage.Object, error) {
	return s.get(c, n)
}
func (s leaseFunctions) Create(c context.Context, n string, b []byte) (string, error) {
	return s.create(c, n, b)
}
func (s leaseFunctions) Replace(c context.Context, n, e string, b []byte) (string, error) {
	return s.replace(c, n, e, b)
}
func leaseFunctionsFor(s *leaseMemory) leaseFunctions {
	return leaseFunctions{s.Get, s.Create, s.Replace}
}
func leaseTime() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
func leaseBudget(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), time.Hour)
	t.Cleanup(cancel)
	return c
}
func leaseManager(t *testing.T, s LeaseObjects, now time.Time) *LeaseManager {
	t.Helper()
	m, e := NewLeaseManager(s, strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	m.now = func() time.Time { return now }
	return m
}
func acquireLease(t *testing.T, m *LeaseManager, c context.Context) *Lease {
	t.Helper()
	l, e := m.Acquire(c)
	if e != nil || l == nil {
		t.Fatalf("acquire: lease=%v error=%v", l, e)
	}
	return l
}
func leaseFailure(t *testing.T, err error, code string) {
	t.Helper()
	var e *LeaseError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error=%v; want %s", err, code)
	}
}
func leaseBody(environment, owner string, at, until time.Time) []byte {
	return []byte(fmt.Sprintf("{\"schemaVersion\":1,\"environmentId\":%q,\"ownerToken\":%q,\"acquiredAt\":%q,\"expiresAt\":%q}", environment, owner, at.Format("2006-01-02T15:04:05.000Z"), until.Format("2006-01-02T15:04:05.000Z")))
}
func seedLease(s *leaseMemory, now time.Time) {
	s.body = leaseBody(strings.Repeat("a", 64), strings.Repeat("b", 64), now, now.Add(600*time.Second))
	s.version = 1
}
func leaseFields(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if e := json.Unmarshal(b, &got); e != nil {
		t.Fatal(e)
	}
	return got
}
