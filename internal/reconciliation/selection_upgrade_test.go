package reconciliation

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
)

func TestSelectedUpgradeHoldsPendingLegacyWorkWithoutRetargeting(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	send := f.p.senders["US"]
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		response, err := send(ctx, body, key)
		response.Status = 503
		return response, err
	}
	if got := f.run(t); got.Deferred != 1 {
		t.Fatal("legacy work not pending", got)
	}
	before := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	setup := &setupRuntimeFixture{objects: f.live.objects, lease: f.live.lease, resourceTypes: []string{"mysqldbsystem"}}
	setup.configure(t, "SEND_TEST", 1)
	var err error
	setup.result, err = setup.coordinator.Run(leaseBudget(t), setup.lease)
	if err != nil {
		t.Fatal(err)
	}
	setup.configure(t, "CONFIRM", 1)
	if _, err = setup.coordinator.Run(leaseBudget(t), setup.lease); err != nil {
		t.Fatal(err)
	}
	configureSelectedProcessing(t, f, setup, []string{"mysqldbsystem"})
	f.p.senders["US"] = send
	got := f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if got.Accepted != 0 || after.State != delivery.ReviewRequired || after.Reason != delivery.ReasonConfigurationChanged || after.AttemptCount != before.AttemptCount || !reflect.DeepEqual(before.Message, after.Message) || !bytes.Equal(before.Message.PayloadWithoutRoutingKey, after.Message.PayloadWithoutRoutingKey) || after.SecretVersion != before.SecretVersion || !after.FirstAttemptAt.Equal(before.FirstAttemptAt) || f.current.Load() != 1 || f.exact.Load() != 3 {
		t.Fatal("upgrade rebuilt or retargeted pending legacy work", got)
	}
	f.run(t)
	if f.current.Load() != 1 || f.sent.Load() != 3 {
		t.Fatal("upgrade sent held batch")
	}
}
