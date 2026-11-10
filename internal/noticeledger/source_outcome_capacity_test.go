package noticeledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

// The decoder intentionally validates the retained source projection, not its
// full original notice digest. This synthetic fixture exercises that wire limit.
func fullSourceRecord(t *testing.T, r Record) Record {
	t.Helper()
	env, id := r.Identity()
	room := (1 << 20) - maximumSize(t, r) - len(`""`) + len("null")
	data := changedWire(t, r, func(w *wireRecord) { w.Revisions[0].Source.Reference = text(strings.Repeat("x", room)) })
	n, err := Decode(data, env, id)
	if err != nil || maximumSize(t, n) != 1<<20 {
		t.Fatal("invalid full-record fixture", err)
	}
	return n
}

func TestSourceOutcomeReservationCoversEveryPermittedReplacement(t *testing.T) {
	stamp := clock().Format(timeFormat)
	maximum, _ := json.Marshal(SourceOutcome{State: SourceAvailable, RecordedAt: stamp, RevisionDigest: strings.Repeat("a", 64)})
	for _, o := range []SourceOutcome{{State: SourceUnknown, RecordedAt: stamp}, {State: SourceUnavailable, RecordedAt: stamp, HTTPStatus: 503}} {
		encoded, _ := json.Marshal(o)
		if len(encoded) > len(maximum) {
			t.Fatal("chosen outcome reservation is not maximal")
		}
	}
	r := observed(t, source(db))
	unknown, err := legacyRecord(t, r).MigrateSourceOutcome(clock())
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []Record{r, unknown, unavailable(t, r, 404, clock())} {
		full := fullSourceRecord(t, base)
		reload(t, unavailable(t, full, 503, clock().Add(time.Second)))
		env, id := full.Identity()
		over := changedWire(t, full, func(w *wireRecord) { *w.Revisions[0].Source.Reference += "x" })
		n, err := Decode(over, env, id)
		rejected(t, n, err)
		var issue *Error
		if !errors.As(err, &issue) || issue.Code != "too_large" {
			t.Fatal("one-byte capacity overflow was not enforced", err)
		}
	}
}

func TestFullLegacyMigrationCannotEraseLateReceiptOrExpiry(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	sending, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	full := fullSourceRecord(t, legacyRecord(t, sending))
	before := encoded(t, full)
	n, err := full.MigrateSourceOutcome(clock())
	rejected(t, n, err)
	var issue *Error
	if !errors.As(err, &issue) || issue.Code != "too_large" || !bytes.Equal(before, encoded(t, full)) {
		t.Fatal("migration did not preserve full legacy state", err)
	}
	accepted, err := full.Finish(ref, 202, clock(), time.Time{})
	if err != nil || accepted.Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("failed migration blocked legacy receipt", err)
	}
	recovered, err := full.Recover(clock().Add(time.Minute))
	if err != nil || !recovered.Snapshot().Batches[0].Ambiguous {
		t.Fatal("failed migration blocked ambiguity recording", err)
	}
	for _, next := range []Record{full, accepted, recovered} {
		if next.Snapshot().SchemaVersion != 1 || next.Snapshot().SourceOutcome != nil {
			t.Fatal("legacy transition migrated implicitly")
		}
		reload(t, next)
	}
}

func TestFullV2SourceOutcomeAndDeliveryTransitionsFit(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	full := fullSourceRecord(t, r)
	lost := unavailable(t, full, 404, clock())
	accepted, err := lost.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := lost.Recover(clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []Record{lost, accepted, recovered} {
		if len(encoded(t, next)) > 1<<20 {
			t.Fatal("admitted transition exceeds storage limit")
		}
		reload(t, next)
	}
}

func TestMigrationReservesFutureAvailableOutcomeBeforeAdmittingUnknown(t *testing.T) {
	full := fullSourceRecord(t, legacyRecord(t, observed(t, source(db))))
	unknown, _ := json.Marshal(SourceOutcome{State: SourceUnknown, RecordedAt: clock().Format(timeFormat)})
	space := len(`,"sourceOutcome":`) + len(unknown)
	env, id := full.Identity()
	body := changedWire(t, full, func(w *wireRecord) {
		value := *w.Revisions[0].Source.Reference
		w.Revisions[0].Source.Reference = text(value[:len(value)-space])
	})
	legacy, err := Decode(body, env, id)
	if err != nil || maximumSize(t, legacy)+space != 1<<20 {
		t.Fatal("invalid migration headroom fixture", err)
	}
	n, err := legacy.MigrateSourceOutcome(clock())
	rejected(t, n, err)
	var issue *Error
	if !errors.As(err, &issue) || issue.Code != "too_large" {
		t.Fatal("migration admitted UNKNOWN without future outcome headroom", err)
	}
}
