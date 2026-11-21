package reconciliation

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/pagerduty"
	"oci-maintenance-router/internal/routing"
)

func TestSelectedComputeRetryPreservesExactSavedRequest(t *testing.T) {
	n := selectedNotice(t, 1, "ACTION_REQUIRED", resourceInstance)
	f, _ := selectedProcessingFixture(t, []string{"instance"}, nil, n)
	send := f.p.senders["US"]
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		response, err := send(ctx, body, key)
		response.Status = 503
		return response, err
	}
	if got := f.run(t); got.Deferred != 1 || len(got.Attempts) != 3 {
		t.Fatal("unexpected retry budget", got)
	}
	before := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	f.p.senders["US"] = send
	if got := f.run(t); got.Accepted != 1 {
		t.Fatal("selected retry failed", got)
	}
	after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if !reflect.DeepEqual(before.Message, after.Message) || before.SecretVersion != after.SecretVersion || !before.FirstAttemptAt.Equal(after.FirstAttemptAt) || after.AttemptCount != 4 || f.current.Load() != 1 || f.exact.Load() != 4 {
		t.Fatal("retry reset saved identity or pin")
	}
}

func TestSelectedComputeCancellationPreventsCredentialReads(t *testing.T) {
	n := selectedNotice(t, 1, "ACTION_REQUIRED", resourceInstance)
	f, _ := selectedProcessingFixture(t, []string{"instance"}, nil, n)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.p.compute["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
		cancel()
		return routing.Metadata{}, context.Canceled
	}
	got, err := f.p.Run(ctx, f.live.lease)
	if !errors.Is(err, context.Canceled) || got.Snapshot().Completed || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("canceled lookup progressed", err)
	}
}
