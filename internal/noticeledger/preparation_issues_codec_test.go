package noticeledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPreparationIssuesCanonicalWireAndDecodedParent(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_invalid", clock(), 0))
	if err != nil {
		t.Fatal(err)
	}
	s := r.Snapshot()
	// Independently spell the wire schema and raw-byte identity tuple. No call to
	// the preparation encoder is used to build expected bytes.
	contextTuple := fmt.Sprintf(`["oci-maintenance-router/preparation-context",1,"%x",{"chainId":null,"reference":null,"times":[{"type":null,"title":null,"value":null},{"type":null,"title":null,"value":null}],"timeCreated":null,"timeUpdated":null}]`, sha256.Sum256(settings(nil)))
	want := fmt.Sprintf(`{"schemaVersion":1,"environmentId":%q,"noticeDigest":"%x","observations":[{"revisionDigest":%q,"resourceIndex":0,"code":"secret_invalid","firstSeenAt":"2026-01-01T00:00:00.000Z","lastSeenAt":"2026-01-01T00:00:00.000Z","contextDigest":"%x","routingGeneration":%q,"destinationId":"payments"}]}`, s.EnvironmentID, sha256.Sum256([]byte(noticeID)), s.Revisions[0].Digest, sha256.Sum256([]byte(contextTuple)), s.Revisions[0].RoutingGeneration)
	body, _ := p.Encode()
	if string(body) != want {
		t.Fatalf("wire mismatch\ngot %s\nwant%s", body, want)
	}
	decoded, err := DecodePreparationIssues(body, r)
	if err != nil || decoded.Follows(PreparationIssues{}) || !p.Follows(PreparationIssues{}) {
		t.Fatal("decoded data became a new proposal", err)
	}
	body[0] = 'x'
	if _, err = decoded.Encode(); err != nil {
		t.Fatal("input bytes aliased record")
	}
}

func TestPreparationIssuesRejectMalformedAndInconsistentStoredData(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_invalid", clock(), 0))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := p.Encode()
	values := [][]byte{nil, []byte("null"), []byte("[]"), []byte("{}"), []byte("{"), []byte{0xff}, append(bytes.Clone(body), '\n'), bytes.Repeat([]byte("["), 33), bytes.Repeat([]byte(" "), recordLimit+1), bytes.Replace(body, []byte(`"resourceIndex":0`), []byte(`"resourceIndex":0.5`), 1)}
	for _, edit := range []func(*PreparationIssueSnapshot){
		func(s *PreparationIssueSnapshot) { s.SchemaVersion = 2 },
		func(s *PreparationIssueSnapshot) { s.EnvironmentID = strings.Repeat("f", 64) },
		func(s *PreparationIssueSnapshot) { s.NoticeDigest = strings.Repeat("f", 64) },
		func(s *PreparationIssueSnapshot) { s.Observations = nil },
		func(s *PreparationIssueSnapshot) { s.Observations[0].ResourceIndex = -1 },
		func(s *PreparationIssueSnapshot) { s.Observations[0].ResourceIndex = 2 },
		func(s *PreparationIssueSnapshot) { s.Observations[0].RevisionDigest = strings.Repeat("f", 64) },
		func(s *PreparationIssueSnapshot) { s.Observations[0].Code = "provider secret detail" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].FirstSeenAt = "2025-01-01T00:00:00.000Z" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].LastSeenAt = "2025-01-01T00:00:00.000Z" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].LastSeenAt = "wrong" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].ContextDigest = "wrong" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].RoutingGeneration = "wrong" },
		func(s *PreparationIssueSnapshot) { s.Observations[0].DestinationID = "BAD" },
		func(s *PreparationIssueSnapshot) { s.Observations = append(s.Observations, s.Observations[0]) },
		func(s *PreparationIssueSnapshot) { s.Observations = make([]PreparationObservation, 2049) },
	} {
		s := p.Snapshot()
		edit(&s)
		b, _ := json.Marshal(s)
		values = append(values, b)
	}
	for i, value := range values {
		got, err := DecodePreparationIssues(value, r)
		if err == nil || got.Snapshot().SchemaVersion != 0 {
			t.Fatal("corrupt state admitted", i)
		}
	}
	if _, err = DecodePreparationIssues(body, Record{}); err == nil {
		t.Fatal("missing authoritative record accepted")
	}
}
