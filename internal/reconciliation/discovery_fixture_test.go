package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

const discoveryCatalog = `{"regions":{"us-ashburn-1":"iad"},"mysqlServices":["Oracle HeatWave MySQL"]}`

func noticeName(id string) string { return fmt.Sprintf("notices/%x.json", sha256.Sum256([]byte(id))) }

func discoveryLegacy(t *testing.T, r noticeledger.Record) noticeledger.Record {
	t.Helper()
	body, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := json.Marshal(r.Snapshot().SourceOutcome)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":1`), 1)
	body = bytes.Replace(body, append([]byte(`,"sourceOutcome":`), outcome...), nil, 1)
	env, id := r.Identity()
	legacy, err := noticeledger.Decode(body, env, id)
	if err != nil {
		t.Fatal(err)
	}
	return legacy
}

func discoveryFixture(t *testing.T, ns []notices.Notice, records []noticeledger.Record, origins ...context.Context) (*Discovery, *discoveryObjects, *Lease) {
	t.Helper()
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	objects.seed("configuration/router.json", []byte(candidateConfig))
	objects.seed("configuration/catalog.json", []byte(discoveryCatalog))
	for _, r := range records {
		body, err := r.Encode()
		if err != nil {
			t.Fatal(err)
		}
		objects.seed(recordName(r), body)
	}
	a, _ := detailSources(t, ns, nil)
	index := 0
	saved := map[string]bool{}
	for _, r := range records {
		_, id := r.Identity()
		saved[id] = true
	}
	a.get = func(_ context.Context, id string) (notices.Notice, error) {
		if index < len(ns) {
			n := ns[index]
			index++
			if n.Key().NoticeID != id {
				t.Fatal("wrong listed detail order")
			}
			return n, nil
		}
		if !saved[id] {
			t.Fatal("unexpected saved source", id)
		}
		delete(saved, id)
		return notices.Notice{}, &ociannouncements.Error{Code: ociannouncements.Service, Status: 404}
	}
	d, err := NewDiscovery([]byte(candidateConfig), []byte(discoveryCatalog), candidateLocation(), a, objects)
	if err != nil {
		t.Fatal("valid durable discovery configuration rejected", err)
	}
	d.wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	now := leaseTime()
	d.now = func() time.Time { return now }
	c, err := configuration.Decode([]byte(candidateConfig), candidateCatalog())
	if err != nil {
		t.Fatal(err)
	}
	env, err := c.EnvironmentID(candidateLocation())
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewLeaseManager(objects, env)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	origin := leaseBudget(t)
	if len(origins) > 0 {
		origin = origins[0]
	}
	return d, objects, acquireLease(t, m, origin)
}

func discoveryFailure(t *testing.T, got DiscoveryResult, err error, code string) {
	t.Helper()
	var detail *DiscoveryError
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v; want%s", err, code)
	}
	if got.EnvironmentID() != "" || got.Candidates().EnvironmentID() != "" || got.Availability().EnvironmentID() != "" || !got.CompletedAt().IsZero() {
		t.Fatal("failed run returned completion evidence")
	}
}
