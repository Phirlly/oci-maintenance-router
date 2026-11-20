package reconciliation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSetupCurrentRejectsZeroOrForeignTransitions(t *testing.T) {
	d, o := setupCurrentIntents(t, 1)
	r, err := newSetupCurrent(1, d, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.selectAttempt(0, d, o); err == nil {
		t.Fatal("invalid successor accepted")
	}
	d2, o2 := d, o
	d2.EnvironmentID, o2.EnvironmentID = strings.Repeat("b", 64), strings.Repeat("b", 64)
	d2.RequestID, o2.RequestID = setupRequestIDs(d2.EnvironmentID, d2.InstallationID, 2)
	if _, err = r.selectAttempt(2, d2, o2); err == nil {
		t.Fatal("foreign successor accepted")
	}
	if _, err = (setupCurrent{}).encode(); err == nil {
		t.Fatal("empty selector serialized")
	}
	s, objects, _ := setupCurrentStoreFixture(t)
	objects.calls = nil
	if _, err = s.create(leaseBudget(t), setupCurrent{}); err == nil {
		t.Fatal("empty proposal created")
	}
	var empty *setupCurrentStore
	if _, _, err = empty.load(leaseBudget(t)); err == nil {
		t.Fatal("unbound store read")
	}
	if len(objects.calls) != 0 {
		t.Fatal("invalid authority reached storage")
	}
}

func TestSetupCurrentRejectsMalformedConfirmationReference(t *testing.T) {
	d, o := setupCurrentIntents(t, 1)
	r, err := newSetupCurrent(1, d, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []setupConfirmationRef{{"not-a-confirmation", strings.Repeat("a", 64)}, {"sc1-" + strings.Repeat("a", 64), "invalid"}} {
		r.data.Confirmation = &ref
		body, _ := json.Marshal(r.data)
		if _, err = decodeSetupCurrent(body, d.EnvironmentID, d.InstallationID); err == nil {
			t.Fatal("invalid reference admitted")
		}
	}
}
