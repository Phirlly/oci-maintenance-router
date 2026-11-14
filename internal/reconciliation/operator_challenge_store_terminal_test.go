package reconciliation

import (
	"bytes"
	"testing"
)

func TestOperatorChallengeStoreRetainsEveryTerminalOutcomeAcrossRestart(t *testing.T) {
	for _, outcome := range []string{"accepted", "unverified", "unknown", "rejected", "expired"} {
		t.Run(outcome, func(t *testing.T) {
			s, objects, backend := operatorStoreFixture(t)
			old := operatorSaved(t, s)
			c := operatorClaimFixture()
			var next OperatorChallenge
			var err error
			switch outcome {
			case "accepted":
				next, err = old.Record().Finish(operatorReference(c), 200, operatorMessageID, c.Now)
			case "unverified":
				next, err = old.Record().Finish(operatorReference(c), 200, "", c.Now)
			case "unknown":
				next, err = old.Record().Finish(operatorReference(c), 0, "", c.Now)
			case "rejected":
				next, err = old.Record().Finish(operatorReference(c), 403, "", c.Now)
			case "expired":
				next, err = old.Record().Expire(c.ExpiresAt)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Save(leaseBudget(t), old, next); err != nil {
				t.Fatal(err)
			}
			restarted := operatorStoreFor(t, backend)
			loaded, err := restarted.Load(leaseBudget(t), "operator-test-1")
			if err != nil {
				t.Fatal(err)
			}
			retained, err := restarted.Create(leaseBudget(t), operatorChallengeFixture(t))
			if err != nil {
				t.Fatal(err)
			}
			want, _ := next.Encode()
			for _, actual := range []StoredOperatorChallenge{loaded, retained} {
				body, _ := actual.Record().Encode()
				if !bytes.Equal(body, want) {
					t.Fatal("restart reset terminal evidence")
				}
			}
			if objects.version != 2 {
				t.Fatal("repeat initialization mutated terminal state")
			}
		})
	}
}

func TestOperatorChallengeNewExplicitRequestUsesSeparateHistory(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	i := operatorIntentFixture(t)
	i.RequestID = "operator-test-2"
	next, err := NewOperatorChallenge(i, operatorClaimFixture())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Create(leaseBudget(t), next)
	if err != nil || !got.Record().Matches(i) {
		t.Fatal("new explicit request rejected", err)
	}
	if len(objects.calls) != 6 || objects.calls[4].method != "HISTORY" || objects.calls[4].name != "control/operator-challenges/installation-1/operator-test-2.json" {
		t.Fatal("new request reused another history")
	}
	retained, err := s.Load(leaseBudget(t), "operator-test-1")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := old.Record().Encode()
	actual, _ := retained.Record().Encode()
	if !bytes.Equal(want, actual) || objects.version != 2 {
		t.Fatal("new request reset preceding claim")
	}
}
