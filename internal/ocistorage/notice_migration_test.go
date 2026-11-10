package ocistorage

import (
	"bytes"
	"encoding/json"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func legacyNotice(t *testing.T) noticeledger.Record {
	t.Helper()
	// Independent original wire layout excludes v2's source outcome. The domain
	// package separately pins historical v1 golden bytes and semantic validation.
	var v1 struct {
		SchemaVersion int             `json:"schemaVersion"`
		EnvironmentID string          `json:"environmentId"`
		NoticeID      string          `json:"noticeId"`
		UpdatedAt     string          `json:"updatedAt"`
		HoldReason    string          `json:"holdReason"`
		Revisions     json.RawMessage `json:"revisions"`
		Delivery      json.RawMessage `json:"delivery"`
	}
	body, err := noticeState(t).Encode()
	if err != nil || json.Unmarshal(body, &v1) != nil {
		t.Fatal("invalid synthetic notice fixture", err)
	}
	v1.SchemaVersion = 1
	body, err = json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := noticeledger.Decode(body, v1.EnvironmentID, v1.NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNoticeSourceMigrationSavesOnlyItsImmediateParent(t *testing.T) {
	c, writes := conditionalServer(t, false)
	r := legacyNotice(t)
	s := noticeStore(t, c, r)
	old, err := s.Create(budget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := old.Record().MigrateSourceOutcome(stateClock())
	if err != nil {
		t.Fatal(err)
	}
	unavailable, err := migration.RecordSourceUnavailable(404, stateClock())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Save(budget(t), old, unavailable)
	wantError(t, err, InvalidRequest, 0, false)
	if got.store != nil || writes.Load() != 1 {
		t.Fatal("grandchild used the original handle")
	}
	saved, err := s.Save(budget(t), old, migration)
	if err != nil || saved.Record().Snapshot().SourceOutcome.State != noticeledger.SourceUnknown {
		t.Fatal("migration save failed", err)
	}
	got, err = s.Save(budget(t), old, migration)
	wantError(t, err, Conflict, 412, false)
	if got.store != nil || got.Record().Snapshot().SourceOutcome != nil {
		t.Fatal("conflict returned usable migrated state")
	}
	if _, err := s.Save(budget(t), saved, unavailable); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(budget(t), stateNoticeID)
	if err != nil || loaded.Record().Snapshot().SourceOutcome.HTTPStatus != 404 {
		t.Fatal("following transition not persisted", err)
	}
}

func TestLostMigrationResponseGrantsNoHandleAndExactReloadRecovers(t *testing.T) {
	c, writes := conditionalServerAt(t, 2)
	r := legacyNotice(t)
	s := noticeStore(t, c, r)
	old, err := s.Create(budget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := old.Record().MigrateSourceOutcome(stateClock())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Save(budget(t), old, migration)
	wantError(t, err, Transport, 0, true)
	if got.store != nil || got.etag != "" || got.Record().Snapshot().SourceOutcome != nil || writes.Load() != 2 {
		t.Fatal("uncertain migration returned handle or retried")
	}
	loaded, err := s.Load(budget(t), stateNoticeID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := migration.Encode()
	actual, _ := loaded.Record().Encode()
	if !bytes.Equal(want, actual) || loaded.Record().Snapshot().SourceOutcome.State != noticeledger.SourceUnknown {
		t.Fatal("exact migration state not recovered")
	}
	stale, err := migration.RecordSourceUnavailable(503, stateClock())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(budget(t), loaded, stale)
	wantError(t, err, InvalidRequest, 0, false)
	fresh, err := loaded.Record().RecordSourceUnavailable(503, stateClock())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(budget(t), loaded, fresh); err != nil || writes.Load() != 3 {
		t.Fatal("fresh proposal did not preserve loaded parent", err)
	}
}
