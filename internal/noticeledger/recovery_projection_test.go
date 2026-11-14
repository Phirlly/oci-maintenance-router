package noticeledger

import (
	"testing"
	"time"
)

func TestRecoveryProjectedReviewMatchesLaterMigrationWithoutSemanticChanges(t *testing.T) {
	old := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	projected, err := old.MigrateRecovery(clock())
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{clock(), clock().Add(time.Hour), clock().Add(48 * time.Hour)} {
		migrated, err := old.MigrateRecovery(at)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []ActionKind{RetryBatch, ApproveRevision, CloseBatch, CloseRevision} {
			target := old.Snapshot().Revisions[0].Digest
			if kind == RetryBatch || kind == CloseBatch {
				target = old.Snapshot().Batches[0].Message.BatchID
			}
			if reviewHash(t, projected, kind, target) != reviewHash(t, migrated, kind, target) {
				t.Fatal("migration time changed reviewed state", kind)
			}
		}
	}
}

func TestRecoveryLegacyProjectionStillBindsSourceAvailability(t *testing.T) {
	old := legacyRecord(t, observed(t, source(db), db))
	unknown, err := old.MigrateSourceOutcome(clock())
	if err != nil {
		t.Fatal(err)
	}
	projected := recoveryRecord(t, unknown)
	id := projected.Snapshot().Revisions[0].Digest
	available, err := observer(t, nil).Observe(projected, normalized(t, source(db)), false, lookups(db), clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if reviewHash(t, projected, CloseRevision, id) == reviewHash(t, available, CloseRevision, id) {
		t.Fatal("UNKNOWN to AVAILABLE silently retained an older review")
	}
}
