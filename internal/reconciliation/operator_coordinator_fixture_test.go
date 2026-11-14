package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

const operatorRunObjectName = "control/operator-challenges/installation-1/operator-1.json"

type operatorCoordinatorFixture struct {
	coordinator  *OperatorCoordinator
	objects      *preparationObjects
	lease        *Lease
	reads, sends atomic.Int32
	publish      func(context.Context, []byte) (ocinotifications.Publication, error)
}

func operatorClient(t *testing.T) *ocinotifications.Client {
	t.Helper()
	c, err := ocinotifications.New("us-ashburn-1", "iad", "ocid1.tenancy.oc1..example", common.NewRawConfigurationProvider("synthetic", "synthetic", "us-ashburn-1", "synthetic", "synthetic", nil))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func operatorCoordinatorFixtureFor(t *testing.T, origins ...context.Context) *operatorCoordinatorFixture {
	t.Helper()
	f := &operatorCoordinatorFixture{objects: &preparationObjects{discoveryObjects: &discoveryObjects{objects: map[string]ocistorage.Object{}}}}
	cfg := diagnosticConfig(t, operatorTestConfig)
	f.objects.seed("configuration/router.json", cfg)
	f.objects.seed("configuration/catalog.json", []byte(preparationCatalog))
	var err error
	f.coordinator, err = NewOperatorCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal("valid operator coordinator rejected", err)
	}
	catalog, _ := configuration.DecodeCatalog([]byte(preparationCatalog))
	config, _ := configuration.Decode(cfg, catalog)
	env, _ := config.EnvironmentID(candidateLocation())
	m, err := NewLeaseManager(f.objects, env)
	if err != nil {
		t.Fatal(err)
	}
	origin := leaseBudget(t)
	if len(origins) > 0 {
		origin = origins[0]
	}
	f.lease = acquireLease(t, m, origin)
	f.publish = func(ctx context.Context, body []byte) (ocinotifications.Publication, error) {
		f.sends.Add(1)
		got := f.saved(t).Snapshot()
		if got.State != OperatorPublishing || !bytes.Equal(body, got.Payload) || ctx.Err() != nil {
			t.Error("publication did not use the confirmed claim and exact saved payload")
		}
		return ocinotifications.Publication{Status: 200, MessageID: "message-1", MayHaveSent: true}, nil
	}
	f.coordinator.read = func(context.Context) (operatorObservation, error) {
		f.reads.Add(1)
		return operatorObservation{f.coordinator.desired, f.publish}, nil
	}
	return f
}

func (f *operatorCoordinatorFixture) saved(t *testing.T) OperatorChallenge {
	t.Helper()
	f.objects.mu.Lock()
	b := bytes.Clone(f.objects.objects["control/operator-challenges/installation-1/"+f.coordinator.intent.RequestID+".json"].Body)
	f.objects.mu.Unlock()
	r, err := DecodeOperatorChallenge(b, f.coordinator.intent.EnvironmentID, "installation-1", f.coordinator.intent.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *operatorCoordinatorFixture) seed(t *testing.T, state OperatorChallengeState, expired bool) OperatorChallenge {
	t.Helper()
	now := f.lease.manager.now().UTC().Truncate(time.Millisecond)
	start := now.Add(-time.Minute)
	if expired {
		start = now.Add(-11 * time.Minute)
	}
	r, err := NewOperatorChallenge(f.coordinator.intent, OperatorClaim{strings.Repeat("c", 64), strings.Repeat("d", 64), start, start.Add(10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if state != OperatorPublishing {
		status, id := 200, "message-old"
		if state == OperatorReview {
			status, id = 400, ""
		}
		r, err = r.Finish(OperatorClaimRef{strings.Repeat("c", 64), strings.Repeat("d", 64)}, status, id, start.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	b, _ := r.Encode()
	f.objects.seed(operatorRunObjectName, b)
	return r
}

func operatorRunFailure(t *testing.T, err error, code string) *OperatorRunError {
	t.Helper()
	var e *OperatorRunError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
	return e
}

func operatorReconfigure(t *testing.T, f *operatorCoordinatorFixture, change func(map[string]any)) {
	t.Helper()
	cfg := diagnosticConfig(t, operatorTestConfig, change)
	d, err := NewOperatorCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	d.read = f.coordinator.read
	f.coordinator = d
	f.objects.seed("configuration/router.json", cfg)
}
