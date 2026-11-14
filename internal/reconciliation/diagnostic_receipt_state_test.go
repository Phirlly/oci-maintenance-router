package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticReceiptRejectsUnusableSavedStateAndWrongCode(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "noncanonical", "tag", "foreign", "SENDING", "REVIEW_REQUIRED", "code"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinatorFixture(t)
			r := f.seed(t, DiagnosticAccepted, false)
			cfg := receiptConfig(t, r)
			want := "invalid_record"
			switch kind {
			case "missing":
				delete(f.objects.objects, diagnosticObjectName)
				want = "read_failed"
			case "corrupt":
				f.objects.seed(diagnosticObjectName, []byte("private"))
			case "noncanonical":
				body, _ := r.Encode()
				f.objects.seed(diagnosticObjectName, append(body, '\n'))
			case "tag":
				body, _ := r.Encode()
				f.objects.objects[diagnosticObjectName] = ocistorage.Object{Body: body, ETag: "W/weak"}
				want = "invalid_response"
			case "foreign":
				d := r.Snapshot()
				d.Intent.EnvironmentID = strings.Repeat("f", 64)
				other, err := NewDiagnostic(d.Intent, d.SecretVersion, DiagnosticClaim{d.OwnerToken, d.ClaimToken, d.StartedAt, d.ExpiresAt})
				if err != nil {
					t.Fatal(err)
				}
				other, err = other.Finish(DiagnosticClaimRef{d.OwnerToken, d.ClaimToken}, 202, d.AcceptedAt)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := other.Encode()
				f.objects.seed(diagnosticObjectName, body)
			case "SENDING", "REVIEW_REQUIRED":
				f.seed(t, DiagnosticState(kind), false)
				want = "not_accepted"
			case "code":
				cfg = receiptConfig(t, r, func(c map[string]any) {
					c["diagnostic"].(map[string]any)["receiptCode"] = "OMR-" + strings.Repeat("0", 32)
				})
				want = "receipt_mismatch"
			}
			v := receiptVerifier(t, f, cfg)
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			receiptFailure(t, got, err, want)
			receiptReadOnly(t, f, before)
			if f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("verification accessed credential/transport")
			}
		})
	}
}
