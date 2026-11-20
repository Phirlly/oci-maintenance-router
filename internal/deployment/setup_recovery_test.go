package deployment

import "testing"

func TestSetupConfigurationPreservesReviewedPausedClosure(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..setup-close", 1, false)
	intent := reviewedClosureIntent(t, f, "c1-setup")
	f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) { setupConfiguration("PAUSED", "NONE")(v); v["recovery"] = intent })
	got := invokeOperational(t, f.h, closeInvocation(f.s, "c1-setup"))
	if got.Status != "closure-completed" || got.Recovery == nil || got.Recovery.Status != "CLOSED" {
		t.Fatal("schema8 blocked reviewed closure", got)
	}
}

func TestSetupSendingRecoveryRequiresRetainedWork(t *testing.T) {
	h, s, objects, _ := liveOperationFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		setupConfiguration("LIVE", "NONE")(v)
		v["recovery"] = sendingRecoveryIntent("RETRY_BATCH")
	})
	got := invokeOperational(t, h, recoverInvocation(s, "r1-reviewed"))
	// Recovery validates the retained target before it checks LIVE readiness.
	if got.Status != "not-ready" || got.Code != "state_failed" || got.ScanCompleted {
		t.Fatal("sending recovery bypassed missing retained work", got)
	}
}
