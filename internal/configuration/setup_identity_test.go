package configuration

import "testing"

func TestSetupControlsPreserveRoutingAndEnvironmentIdentity(t *testing.T) {
	legacy := decoded(t, preparationSettings(t))
	want, err := legacy.RoutingGeneration()
	if err != nil {
		t.Fatal(err)
	}
	location := LedgerLocation{Namespace: "example", Bucket: "router-state"}
	environment, err := legacy.EnvironmentID(location)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ mode, action string }{{"PREVIEW", "NONE"}, {"PAUSED", "NONE"}, {"TEST", "SEND_TEST"}, {"TEST", "CONFIRM"}, {"LIVE", "NONE"}} {
		c := decoded(t, setupSettings(t, tc.mode, tc.action))
		got, err := c.RoutingGeneration()
		if err != nil || got != want {
			t.Fatal("setup changed existing routing identity", err)
		}
		got, err = c.EnvironmentID(location)
		if err != nil || got != environment {
			t.Fatal("setup changed ledger identity", err)
		}
	}
}
