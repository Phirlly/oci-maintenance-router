package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestAccepted202ReceiptStillRequiresCurrentAuthority(t *testing.T) {
	for _, kind := range []string{"code", "configuration", "catalog", "lease", "channel", "inactive"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorStatusFixture(t, 202)
			accepted, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil || accepted.Record().Snapshot().State != OperatorAccepted {
				t.Fatal("202 acceptance prerequisite", err)
			}
			cfg := operatorReceiptConfig(t, accepted.Record(), func(c map[string]any) {
				if kind == "code" {
					c["operatorTest"].(map[string]any)["receiptCode"] = "OMR-OP-00000000000000000000000000000000"
				}
			})
			v := operatorReceiptVerifier(t, f, cfg)
			want := "receipt_mismatch"
			original := v.read
			v.read = func(ctx context.Context) (OperatorChannel, error) {
				got, err := original(ctx)
				switch kind {
				case "configuration":
					f.objects.seed("configuration/router.json", []byte("changed"))
				case "catalog":
					f.objects.seed("configuration/catalog.json", []byte("changed"))
				case "lease":
					f.objects.seed(workerLeaseName, f.lease.record.bytes())
				case "channel":
					got.Endpoint = "different@example.invalid"
				case "inactive":
					return got, &ocinotifications.Error{Code: ocinotifications.InactiveChannel, Status: 200}
				}
				return got, err
			}
			switch kind {
			case "configuration", "catalog":
				want = "configuration_changed"
			case "lease":
				want = "ownership_lost"
			case "channel":
				want = "channel_changed"
			case "inactive":
				want = "channel_read"
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			operatorReceiptFailure(t, got, err, want)
			operatorReceiptReadOnly(t, f, before)
			if f.sends.Load() != 1 {
				t.Fatal("receipt retried publication")
			}
		})
	}
}
