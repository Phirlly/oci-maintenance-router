package reconciliation

import (
	"bytes"
	"context"
	"testing"

	"oci-maintenance-router/internal/ociownership"
	"oci-maintenance-router/internal/routing"
)

func TestSelectedMixedTeamsReceiveOnlyTheirResources(t *testing.T) {
	n := selectedNotice(t, 1, "ACTION_REQUIRED", processingDB, resourceInstance)
	f, setup := selectedProcessingFixture(t, []string{"mysqldbsystem", "instance"}, nil, n)
	configureSelectedProcessing(t, f, setup, []string{"mysqldbsystem", "instance"}, func(v map[string]any) {
		v["destinations"] = []any{map[string]any{"id": "payments", "secretId": "ocid1.vaultsecret.oc1.iad.payments"}, map[string]any{"id": "video", "secretId": "ocid1.vaultsecret.oc1.iad.video"}}
		v["routes"] = []any{map[string]any{"ownerValue": "payments", "destinationId": "payments"}, map[string]any{"ownerValue": "video", "destinationId": "video"}}
	})
	lookup := f.p.compute["us-ashburn-1"]
	f.p.compute["us-ashburn-1"] = func(ctx context.Context, id string) (routing.Metadata, error) {
		m, err := lookup(ctx, id)
		m.FreeformTags["owner"] = "video"
		return m, err
	}
	if got := f.run(t); got.Accepted != 2 || f.sent.Load() != 2 {
		t.Fatal("teams not delivered separately", got)
	}
	for _, batch := range f.saved(t, n.Key().NoticeID).Snapshot().Batches {
		want, other := processingDB, resourceInstance
		if batch.Message.DestinationID == "video" {
			want, other = other, want
		}
		if len(batch.Message.Resources) != 1 || batch.Message.Resources[0].ResourceID != want || bytes.Contains(batch.Message.PayloadWithoutRoutingKey, []byte(other)) {
			t.Fatal("cross-team resource disclosure")
		}
	}
}

func TestSelectedComputeLookupFailuresNeverReadSecretsOrSend(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		n := selectedNotice(t, 1, "ACTION_REQUIRED", resourceInstance)
		f, _ := selectedProcessingFixture(t, []string{"instance"}, nil, n)
		f.p.compute["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
			return routing.Metadata{}, &ociownership.Error{Code: ociownership.Service, Status: status}
		}
		got := f.run(t)
		row := f.saved(t, n.Key().NoticeID).Snapshot().Revisions[0].Resources[0]
		if got.Accepted != 0 || len(got.Issues) != 1 || row.Disposition != "UNRESOLVED" || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("Compute failure escaped unresolved state", got)
		}
	}
}
