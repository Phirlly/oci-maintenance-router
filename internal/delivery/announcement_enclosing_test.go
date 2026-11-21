package delivery_test

import (
	"bytes"
	"os"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestAnnouncementDeliveryRequiresSeparateEnclosingAdoption(t *testing.T) {
	r, _ := prepared(t, db)
	r = upgraded(t, r)
	next, err := r.MigrateAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	nested := encoded(t, next)
	s := next.Batches()[0].Message
	if _, err := delivery.DecodeRecord(nested, s.EnvironmentID, s.Revision.NoticeID); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"source-only.json", "source-only-v2.json", "source-only-v3.json"} {
		body, err := os.ReadFile("../noticeledger/testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.TrimSpace(body)
		body = bytes.ReplaceAll(body, []byte("c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"), []byte(s.EnvironmentID))
		if _, err := noticeledger.Decode(body, s.EnvironmentID, s.Revision.NoticeID); err != nil {
			t.Fatal("invalid outer fixture", err)
		}
		body = bytes.Replace(body, []byte(`"delivery":null`), append([]byte(`"delivery":`), nested...), 1)
		if _, err := noticeledger.Decode(body, s.EnvironmentID, s.Revision.NoticeID); err == nil || err.Error() != "notice ledger: invalid_delivery" {
			t.Fatal("outer codec admitted delivery3 or failed for wrong reason", err)
		}
	}
}
