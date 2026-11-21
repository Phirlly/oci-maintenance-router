package noticeledger

import (
	"bytes"
	"testing"
)

func TestSelectionDispositionsRequireAnnouncementFormat(t *testing.T) {
	for _, tc := range []struct {
		kinds               []string
		services            []string
		disposition, reason string
	}{
		{[]string{"instance"}, []string{selectedComputeService}, "OUT_OF_SCOPE", "resource_type_out_of_scope"},
		{[]string{"mysqldbsystem"}, []string{selectedComputeService}, "UNRESOLVED", "unsupported_service"},
	} {
		src := source(db)
		src.Services = tc.services
		r, err := selectedObserver(t, tc.kinds...).Observe(Record{}, normalized(t, src), true, nil, clock())
		if err != nil {
			t.Fatal("selection disposition rejected", err)
		}
		row := r.Snapshot().Revisions[0].Resources[0]
		if row.Disposition != tc.disposition || row.Reason != tc.reason || row.ResourceID != db || row.Region != "us-ashburn-1" {
			t.Fatal("wrong selection disposition", row)
		}
		b := encoded(t, r)
		reload(t, r)
		legacy := bytes.Replace(b, []byte(`"schemaVersion":4`), []byte(`"schemaVersion":3`), 1)
		_, err = Decode(legacy, r.Snapshot().EnvironmentID, r.Snapshot().NoticeID)
		if err == nil {
			t.Fatal("new disposition accepted by legacy codec")
		}
	}
}
