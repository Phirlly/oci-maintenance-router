package ocistorage

import (
	"bytes"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func recoveryNotice(t *testing.T) noticeledger.Record {
	t.Helper()
	r, err := noticeState(t).MigrateRecovery(stateClock())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNoticeAnnouncementMigrationRequiresImmediateParentAndCurrentETag(t *testing.T) {
	c, writes := conditionalServer(t, false)
	r := recoveryNotice(t)
	s := noticeStore(t, c, r)
	old, err := s.Create(budget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := old.Record().MigrateAnnouncements(stateClock())
	if err != nil {
		t.Fatal(err)
	}
	child, err := migration.RecordSourceUnavailable(404, stateClock())
	if err != nil {
		t.Fatal(err)
	}
	bad, err := s.Save(budget(t), old, child)
	wantError(t, err, InvalidRequest, 0, false)
	if bad.store != nil || writes.Load() != 1 {
		t.Fatal("grandchild saved against old handle")
	}
	saved, err := s.Save(budget(t), old, migration)
	if err != nil || saved.Record().Snapshot().SchemaVersion != 4 {
		t.Fatal(err)
	}
	bad, err = s.Save(budget(t), old, migration)
	wantError(t, err, Conflict, 412, false)
	if bad.store != nil || bad.Record().Snapshot().SchemaVersion != 0 {
		t.Fatal("stale ETag yielded handle")
	}
	if _, err := s.Save(budget(t), saved, child); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(budget(t), stateNoticeID)
	if err != nil || loaded.Record().Snapshot().SchemaVersion != 4 || loaded.Record().Snapshot().SourceOutcome.HTTPStatus != 404 {
		t.Fatal("following write lost", err)
	}
}

func TestUncertainAnnouncementMigrationRequiresExactReloadAndFreshProposal(t *testing.T) {
	c, writes := conditionalServerAt(t, 2)
	r := recoveryNotice(t)
	s := noticeStore(t, c, r)
	old, err := s.Create(budget(t), r)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := old.Record().MigrateAnnouncements(stateClock())
	if err != nil {
		t.Fatal(err)
	}
	bad, err := s.Save(budget(t), old, migration)
	wantError(t, err, Transport, 0, true)
	if bad.store != nil || bad.etag != "" || bad.Record().Snapshot().SchemaVersion != 0 || writes.Load() != 2 {
		t.Fatal("uncertain save yielded handle or retried")
	}
	loaded, err := s.Load(budget(t), stateNoticeID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := migration.Encode()
	got, _ := loaded.Record().Encode()
	if !bytes.Equal(want, got) {
		t.Fatal("reload changed migration")
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
		t.Fatal("fresh proposal failed", err)
	}
}
