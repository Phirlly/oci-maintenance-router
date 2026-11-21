package reconciliation

import (
	"testing"

	"oci-maintenance-router/internal/notices"
)

func TestCollectionKeepsMySQLCandidateAfterUnrelatedNullableNotice(t *testing.T) {
	unrelated := normalizeCandidate(t, notices.Record{
		NoticeID: noticeID(1), AnnouncementType: "ACTION_REQUIRED", Services: []string{"Oracle Integration"},
		Resources: []notices.Resource{{ResourceID: "unrelated-resource", AdditionalProperties: []notices.Property{{Name: chain("description"), Value: nil}}}},
	})
	mysql := normalizeCandidate(t, notices.Record{
		NoticeID: noticeID(2), AnnouncementType: "SCHEDULED_MAINTENANCE", Services: []string{"Oracle HeatWave MySQL"},
		Resources: []notices.Resource{{ResourceID: "ocid1.mysqldbsystem.oc1.iad.example"}},
	})
	a, s := detailSources(t, []notices.Notice{unrelated, mysql}, nil)
	got, err := collector(t, a, s).collect(budget(t), noPause)
	if err != nil || len(got.Current) != 2 || len(got.Observations) != 1 || len(got.Saved) != 0 {
		t.Fatalf("nullable unrelated notice blocked complete collection: %+v %v", got, err)
	}
	if got.Current[0].Revision != unrelated.Key() || got.Current[0].KnownCandidate || got.Current[1].Revision != mysql.Key() || !got.Current[1].KnownCandidate {
		t.Fatal("unrelated metadata changed candidate membership")
	}
}
