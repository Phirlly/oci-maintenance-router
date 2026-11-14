package noticeledger

import (
	"bytes"
	"testing"
)

func TestLegacyNoticeRejectsNewDeliveryVersionBeforeOuterAdoption(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	for _, old := range []Record{legacyRecord(t, r), r} {
		body := changedWire(t, old, func(w *wireRecord) {
			w.Delivery = bytes.Replace(w.Delivery, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1)
			w.Delivery = bytes.Replace(w.Delivery, []byte(`"ambiguous":false`), []byte(`"ambiguous":false,"attemptKind":""`), 1)
		})
		env, id := old.Identity()
		got, err := Decode(body, env, id)
		rejected(t, got, err)
		reload(t, old)
	}
}
