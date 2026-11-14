package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorReceiptRejectsChangedAuthorityBeforeAndAfterRead(t *testing.T) {
	for _, stage := range []string{"initial", "challenge", "channel", "final-config"} {
		for _, control := range []string{"config", "catalog", "lease"} {
			// Changes after that control's last read are the documented non-atomic
			// observation limit, not a guarantee of a transactional snapshot.
			if stage == "final-config" && control != "catalog" {
				continue
			}
			t.Run(stage+"/"+control, func(t *testing.T) {
				f := operatorCoordinatorFixtureFor(t)
				v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
				change := func() {
					switch control {
					case "config":
						f.objects.seed("configuration/router.json", []byte("changed"))
					case "catalog":
						f.objects.seed("configuration/catalog.json", []byte("changed"))
					case "lease":
						f.objects.seed(workerLeaseName, f.lease.record.bytes())
					}
				}
				if stage == "initial" {
					change()
				}
				if stage == "channel" {
					old := v.read
					v.read = func(c context.Context) (OperatorChannel, error) { got, err := old(c); change(); return got, err }
				}
				configs := 0
				f.objects.after = func(_ context.Context, c discoveryCall) error {
					if c.name == "configuration/router.json" {
						configs++
					}
					if stage == "challenge" && c.name == operatorRunObjectName || stage == "final-config" && c.name == "configuration/router.json" && configs == 2 {
						change()
					}
					return nil
				}
				before := len(f.objects.calls)
				got, err := v.Verify(leaseBudget(t), f.lease)
				want := "configuration_changed"
				if control == "lease" {
					want = "ownership_lost"
				}
				operatorReceiptFailure(t, got, err, want)
				operatorReceiptReadOnly(t, f, before)
			})
		}
	}
}

func TestOperatorReceiptCannotTrustBackendErrorsOrMissingControls(t *testing.T) {
	for _, name := range []string{"configuration/router.json", "configuration/catalog.json", operatorRunObjectName} {
		for _, failure := range []error{errors.New("private"), &OperatorRunError{Code: "receipt_mismatch", HTTPStatus: 202}, &ocistorage.Error{Code: ocistorage.Service, Status: 403}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404})} {
			f := operatorCoordinatorFixtureFor(t)
			v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
			f.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.name == name {
					return failure
				}
				return nil
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			want := "configuration_read"
			if name == operatorRunObjectName {
				want = "read_failed"
			}
			e := operatorReceiptFailure(t, got, err, want)
			wantStatus := 0
			if x, ok := failure.(*ocistorage.Error); ok {
				wantStatus = x.Status
			}
			if e.HTTPStatus != wantStatus || e.WriteUncertain || strings.Contains(err.Error(), "private") {
				t.Fatal("backend forged evidence or leaked details")
			}
			operatorReceiptReadOnly(t, f, before)
		}
	}
}
