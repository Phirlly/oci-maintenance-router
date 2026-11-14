package reconciliation

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestAcceptanceFreshWorkersPreserveMixedOutcomesAndRecoverOnlyOutstandingWork(t *testing.T) {
	oldDB := "ocid1.mysqldbsystem.oc1.iad.baseline"
	old := processingNotice(t, 2, oldDB)
	s := acceptanceSetup(t, old)
	a, b, c := processingDB, "ocid1.mysqldbsystem.oc1.iad.video", "ocid1.mysqldbsystem.oc1.iad.blocked"
	u, outside := "ocid1.mysqldbsystem.oc1.iad.unknown", "ocid1.mysqldbsystem.oc1.iad.outside"
	n := processingNotice(t, 1, a, b, c, u, outside)
	s.notices[n.Key().NoticeID] = n
	s.owners = map[string]string{oldDB: "payments", a: "payments", b: "video", c: "blocked", outside: "outside"}
	s.statuses["video"] = 400
	s.blockedSecret = "ocid1.vaultsecret.oc1.iad.blocked"
	first := s.run(t)
	if first.Prepared != 2 || first.Accepted != 1 || first.Suppressed != 1 || len(s.sends) != 2 {
		t.Fatal("wrong first-pass accounting", first)
	}
	for _, send := range s.sends {
		want := a
		if send.destination == "video" {
			want = b
		} else if send.destination != "payments" {
			t.Fatal("unrelated destination sent", send.destination)
		}
		if !reflect.DeepEqual(send.resources, []string{want}) || send.version != 7 {
			t.Fatal("recipient leaked another owner's resources or used wrong key")
		}
	}
	before := s.saved(t, n.Key().NoticeID)
	accepted, failed := acceptanceBatch(t, before, "payments"), acceptanceBatch(t, before, "video")
	if accepted.State != delivery.APIAccepted || failed.State != delivery.ReviewRequired || failed.Reason != delivery.ReasonPermanent {
		t.Fatal("mixed outcomes were not durably separated")
	}
	view := s.review(t, n.Key().NoticeID)
	if len(view.Preparation) != 1 || view.Preparation[0].Status != "LAST_BLOCKED" || view.Preparation[0].Observation.Code != "secret_unavailable" {
		t.Fatal("preparation failure lacks current durable evidence", view)
	}
	unknown := false
	for _, issue := range view.Issues {
		unknown = unknown || issue.Kind == "RESOURCE" && issue.Code == "tag_missing"
	}
	if !unknown || slices.Contains(s.currentReads, "ocid1.vaultsecret.oc1.iad.unrelated") {
		t.Fatal("unknown ownership hidden or unrelated credential read")
	}
	reads := slices.Clone(s.currentReads)
	slices.Sort(reads)
	if !reflect.DeepEqual(reads, []string{"ocid1.vaultsecret.oc1.iad.blocked", "ocid1.vaultsecret.oc1.iad.payments", "ocid1.vaultsecret.oc1.iad.video"}) {
		t.Fatal("CURRENT reads do not match the three new destinations", reads)
	}
	observation := view.Preparation[0].Observation
	request := reviewedRecovery(t, before, noticeledger.RetryBatch, failed.Message.BatchID)
	s.recovery(t, request)
	s.reconstruct(t)
	s.blockedSecret, s.currentVersion, s.statuses["video"] = "", 9, 202
	startReads, startSends := len(s.currentReads), len(s.sends)
	second := s.run(t)
	after := s.saved(t, n.Key().NoticeID)
	if second.Accepted != 2 || second.Prepared != 1 || second.Recovery == nil || second.Recovery.Status != string(noticeledger.RecoveryAccepted) {
		t.Fatal("outstanding work did not recover", second)
	}
	if !reflect.DeepEqual(accepted, acceptanceBatch(t, after, "payments")) {
		t.Fatal("accepted receipt changed during recovery migration")
	}
	retried := acceptanceBatch(t, after, "video")
	newlyAssigned := acceptanceBatch(t, after, "blocked")
	if len(after.Snapshot().Batches) != 3 || newlyAssigned.State != delivery.APIAccepted || newlyAssigned.SecretVersion != 9 {
		t.Fatal("restored secret did not produce a new confirmed recipient")
	}
	if !reflect.DeepEqual(retried.Message, failed.Message) || retried.SecretVersion != 7 || retried.AttemptCount != failed.AttemptCount+1 || !retried.FirstAttemptAt.Equal(failed.FirstAttemptAt) {
		t.Fatal("manual retry rebuilt immutable identity or reset provenance")
	}
	if !reflect.DeepEqual(s.currentReads[startReads:], []string{"ocid1.vaultsecret.oc1.iad.blocked"}) || len(s.sends)-startSends != 2 {
		t.Fatal("retry followed CURRENT or replayed accepted work")
	}
	for _, send := range s.sends[startSends:] {
		if send.destination == "video" && (!bytes.Equal(send.payload, failed.Message.PayloadWithoutRoutingKey) || send.version != 7 || !reflect.DeepEqual(send.resources, []string{b})) {
			t.Fatal("manual dispatch differs from saved delivery")
		}
		if send.destination == "blocked" && (send.version != 9 || !reflect.DeepEqual(send.resources, []string{c})) {
			t.Fatal("newly prepared work did not use current secret/own members")
		}
	}
	view = s.review(t, n.Key().NoticeID)
	if len(view.Preparation) != 1 || view.Preparation[0].Status != "HISTORICAL" || view.Preparation[0].Observation != observation {
		t.Fatal("assignment erased failure history or failed to classify it")
	}
	receipts := after.Snapshot().Batches
	s.reconstruct(t)
	startSends = len(s.sends)
	third := s.run(t)
	if third.Accepted != 0 || third.Prepared != 0 || len(s.sends) != startSends || !reflect.DeepEqual(receipts, s.saved(t, n.Key().NoticeID).Snapshot().Batches) {
		t.Fatal("fresh worker replayed request or mutated receipts")
	}
	// A replacement uses the same ownership value without a static resource list.
	replacement := "ocid1.mysqldbsystem.oc1.iad.replacement"
	arrival := processingNotice(t, 3, replacement)
	s.notices[arrival.Key().NoticeID], s.owners[replacement] = arrival, "payments"
	s.reconstruct(t)
	fourth := s.run(t)
	if fourth.Accepted != 1 || len(s.sends) != startSends+1 || !reflect.DeepEqual(s.sends[len(s.sends)-1].resources, []string{replacement}) || s.sends[len(s.sends)-1].destination != "payments" {
		t.Fatal("replacement resource did not follow current owner", fourth)
	}
	if !reflect.DeepEqual(receipts, s.saved(t, n.Key().NoticeID).Snapshot().Batches) {
		t.Fatal("replacement processing altered prior receipts")
	}
}
