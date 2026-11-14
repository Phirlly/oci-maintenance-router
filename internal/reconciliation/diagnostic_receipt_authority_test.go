package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticReceiptRejectsChangedAuthorityBeforeAndAfterRead(t *testing.T) {
	for _, stage := range []string{"initial", "diagnostic", "final-config"} {
		for _, control := range []string{"config", "catalog", "lease"} {
			// Changes after that control's last read are the documented non-atomic
			// observation limit, not a guarantee of a transactional snapshot.
			if stage == "final-config" && control != "catalog" {
				continue
			}
			t.Run(stage+"/"+control, func(t *testing.T) {
				f := coordinatorFixture(t)
				v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
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
				configs := 0
				f.objects.after = func(_ context.Context, c discoveryCall) error {
					if c.name == "configuration/router.json" {
						configs++
					}
					if stage == "diagnostic" && c.name == diagnosticObjectName || stage == "final-config" && c.name == "configuration/router.json" && configs == 2 {
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
				receiptFailure(t, got, err, want)
				receiptReadOnly(t, f, before)
			})
		}
	}
}

func TestDiagnosticReceiptCannotTrustBackendErrorsOrMissingControls(t *testing.T) {
	for _, name := range []string{"configuration/router.json", "configuration/catalog.json", diagnosticObjectName} {
		for _, failure := range []error{errors.New("private"), &DiagnosticRunError{Code: "receipt_mismatch", HTTPStatus: 202}, &ocistorage.Error{Code: ocistorage.Service, Status: 403}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404})} {
			f := coordinatorFixture(t)
			v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
			f.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.name == name {
					return failure
				}
				return nil
			}
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			want := "configuration_read"
			if name == diagnosticObjectName {
				want = "read_failed"
			}
			e := receiptFailure(t, got, err, want)
			wantStatus := 0
			if x, ok := failure.(*ocistorage.Error); ok {
				wantStatus = x.Status
			}
			if e.HTTPStatus != wantStatus || e.WriteUncertain || strings.Contains(err.Error(), "private") {
				t.Fatal("backend forged evidence or leaked details")
			}
			receiptReadOnly(t, f, before)
		}
	}
}
