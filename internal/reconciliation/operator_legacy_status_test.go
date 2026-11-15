package reconciliation

import (
	"bytes"
	"os"
	"testing"
)

func TestLegacyOperator202ReviewRemainsCanonicalAndTerminal(t *testing.T) {
	body, err := os.ReadFile("testdata/operator-review-202.json")
	if err != nil {
		t.Fatal(err)
	}
	s, objects, backend := operatorStoreFixture(t)
	objects.seed(operatorObjectName, body)
	versions := objects.version
	for range 2 {
		loaded, err := s.Load(leaseBudget(t), "operator-test-1")
		if err != nil {
			t.Fatal(err)
		}
		got, err := loaded.Record().Expire(operatorClaimFixture().ExpiresAt)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := got.Encode()
		state := got.Snapshot()
		if err != nil || !bytes.Equal(encoded, body) || state.State != OperatorReview || state.Reason != "response" || state.HTTPStatus != 202 || !state.Ambiguous || state.MessageID != "" || !state.AcceptedAt.IsZero() {
			t.Fatal("legacy review changed or became accepted", err)
		}
		next, err := got.Finish(operatorReference(operatorClaimFixture()), 202, "new-id", operatorClaimFixture().ExpiresAt)
		operatorRejected(t, next, err, "terminal")
		retained, err := s.Create(leaseBudget(t), operatorChallengeFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err = retained.Record().Encode()
		if err != nil || !bytes.Equal(encoded, body) || objects.version != versions {
			t.Fatal("initialization rewrote consumed legacy request", err)
		}
		s = operatorStoreFor(t, backend)
	}
}

func TestLegacyOperator202CoordinatorNeverRepublishesOrGrantsReceipt(t *testing.T) {
	for _, expired := range []bool{false, true} {
		f := operatorCoordinatorFixtureFor(t)
		old := f.seed(t, OperatorReview, expired)
		body, err := old.Encode()
		if err != nil {
			t.Fatal(err)
		}
		// Preserve the pre-change review shape; do not generate it with Finish202.
		body = bytes.Replace(body, []byte(`"httpStatus":400`), []byte(`"httpStatus":202`), 1)
		body = bytes.Replace(body, []byte(`"ambiguous":false`), []byte(`"ambiguous":true`), 1)
		f.objects.seed(operatorRunObjectName, body)
		versions := f.objects.version
		for range 2 {
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			encoded, encodeErr := got.Record().Encode()
			if err != nil || encodeErr != nil || got.Dispatched() || got.ObservedStatus() != 0 || !bytes.Equal(encoded, body) || f.reads.Load() != 0 || f.sends.Load() != 0 || f.objects.version != versions {
				t.Fatal("legacy request read channel, sent or changed", err, encodeErr)
			}
		}
		v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.saved(t)))
		before := len(f.objects.calls)
		receipt, err := v.Verify(leaseBudget(t), f.lease)
		operatorReceiptFailure(t, receipt, err, "not_accepted")
		operatorReceiptReadOnly(t, f, before)
		if f.reads.Load() != 0 || f.sends.Load() != 0 {
			t.Fatal("legacy review granted channel or publication authority")
		}
	}
}
