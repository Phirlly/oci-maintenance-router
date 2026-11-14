package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorReceiptRejectsUnusableSavedStateAndWrongCode(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "noncanonical", "tag", "foreign", "PUBLISHING", "REVIEW_REQUIRED", "code"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			r := f.seed(t, OperatorAccepted, false)
			cfg := operatorReceiptConfig(t, r)
			want := "invalid_record"
			switch kind {
			case "missing":
				delete(f.objects.objects, operatorRunObjectName)
				want = "read_failed"
			case "corrupt":
				f.objects.seed(operatorRunObjectName, []byte("private"))
			case "noncanonical":
				body, _ := r.Encode()
				f.objects.seed(operatorRunObjectName, append(body, '\n'))
			case "tag":
				body, _ := r.Encode()
				f.objects.objects[operatorRunObjectName] = ocistorage.Object{Body: body, ETag: "W/weak"}
				want = "invalid_response"
			case "foreign":
				d := r.Snapshot()
				d.Intent.EnvironmentID = strings.Repeat("f", 64)
				other, err := NewOperatorChallenge(d.Intent, OperatorClaim{d.OwnerToken, d.ClaimToken, d.StartedAt, d.ExpiresAt})
				if err != nil {
					t.Fatal(err)
				}
				other, err = other.Finish(OperatorClaimRef{d.OwnerToken, d.ClaimToken}, 200, "message-other", d.AcceptedAt)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := other.Encode()
				f.objects.seed(operatorRunObjectName, body)
			case "PUBLISHING", "REVIEW_REQUIRED":
				f.seed(t, OperatorChallengeState(kind), false)
				want = "not_accepted"
			case "code":
				cfg = operatorReceiptConfig(t, r, func(c map[string]any) {
					c["operatorTest"].(map[string]any)["receiptCode"] = "OMR-OP-" + strings.Repeat("0", 32)
				})
				want = "receipt_mismatch"
			}
			v := operatorReceiptVerifier(t, f, cfg)
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			operatorReceiptFailure(t, got, err, want)
			operatorReceiptReadOnly(t, f, before)
			if f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("verification accessed credential/transport")
			}
		})
	}
}
