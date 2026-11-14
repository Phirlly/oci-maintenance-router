package reconciliation

import (
	"bytes"
	"context"
	"encoding/json"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"testing"
	"time"
)

func TestRecoveryLegacyClosureConfirmsBothMigrationsSeparately(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	body, _ := old.Encode()
	outcome, _ := json.Marshal(old.Snapshot().SourceOutcome)
	legacyBytes := bytes.Replace(body, append([]byte(`,"sourceOutcome":`), outcome...), nil, 1)
	legacyBytes = bytes.Replace(legacyBytes, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":1`), 1)
	legacy, err := noticeledger.Decode(legacyBytes, f.live.lease.manager.environmentID, n.Key().NoticeID)
	if err != nil || legacy.Snapshot().SchemaVersion != 1 {
		t.Fatal("invalid legacy fixture", err)
	}
	projected, err := legacy.MigrateSourceOutcome(time.Now())
	if err == nil {
		projected, err = projected.MigrateRecovery(time.Now())
	}
	if err != nil {
		t.Fatal(err)
	}
	q := reviewedRecovery(t, projected, noticeledger.CloseRevision, n.Key().Digest)
	f.live.objects.seed(noticeName(n.Key().NoticeID), legacyBytes)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	versions, tags := []int{}, []string{}
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == noticeName(n.Key().NoticeID) {
			r, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, n.Key().NoticeID)
			if err != nil {
				t.Fatal(err)
			}
			versions = append(versions, r.Snapshot().SchemaVersion)
			tags = append(tags, c.tag)
		}
		return nil
	}
	r, err := p.Run(leaseBudget(t), f.live.lease)
	if err != nil || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Status != string(noticeledger.RecoveryClosed) || len(versions) != 3 || versions[0] != 2 || versions[1] != 3 || versions[2] != 3 || tags[0] == tags[1] || tags[1] == tags[2] {
		t.Fatal("legacy transitions lost their immediate parents", err, versions, tags)
	}
}

func TestRecoveryMigrationRejectsRecordFromFutureWithoutChangingIt(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	at := time.Now().Add(time.Minute)
	old := f.seedBatch(t, n, at, delivery.Prepared, true)
	projected, err := old.MigrateRecovery(at)
	if err != nil {
		t.Fatal(err)
	}
	q := reviewedRecovery(t, projected, noticeledger.CloseRevision, n.Key().Digest)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	r, err := p.Run(leaseBudget(t), f.live.lease)
	before, _ := old.Encode()
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid migration clock overwrote saved history", err)
	}
}
