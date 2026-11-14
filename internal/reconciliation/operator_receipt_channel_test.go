package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestOperatorReceiptRequiresExactCurrentChannel(t *testing.T) {
	for _, kind := range []string{"topic", "subscription", "protocol", "endpoint", "pending", "deleted", "error", "typed-nil", "forged", "status"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
			v.read = func(context.Context) (OperatorChannel, error) {
				c := v.input.desired
				switch kind {
				case "topic":
					c.TopicID += "other"
				case "subscription":
					c.SubscriptionID += "other"
				case "protocol":
					c.Protocol = "HTTPS"
				case "endpoint":
					c.Endpoint = "team@example.invalid"
				case "pending":
					return c, &ocinotifications.Error{Code: ocinotifications.InvalidResponse, Status: 200}
				case "deleted":
					return c, &ocinotifications.Error{Code: ocinotifications.Service, Status: 404}
				case "error":
					return c, errors.New("private contact")
				case "typed-nil":
					return c, (*ocinotifications.Error)(nil)
				case "forged":
					return c, &OperatorRunError{Code: "private contact", HTTPStatus: 200}
				case "status":
					return c, &ocinotifications.Error{Code: ocinotifications.Service, Status: 999}
				}
				return c, nil
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			want := "channel_read"
			if kind == "topic" || kind == "subscription" || kind == "protocol" || kind == "endpoint" {
				want = "channel_changed"
			}
			e := operatorReceiptFailure(t, got, err, want)
			status := 0
			if kind == "pending" {
				status = 200
			}
			if kind == "deleted" {
				status = 404
			}
			if e.HTTPStatus != status || e.WriteUncertain || strings.Contains(err.Error(), "private") || f.sends.Load() != 0 {
				t.Fatal("invalid channel leaked evidence or published")
			}
			operatorReceiptReadOnly(t, f, before)
		})
	}
}
