package reconciliation

import (
	"bytes"
	"sync"
	"testing"
)

func TestOperatorChallengeSnapshotsAndRestartOwnBytes(t *testing.T) {
	r := operatorChallengeFixture(t)
	body, _ := r.Encode()
	original := bytes.Clone(body)
	i := r.Snapshot().Intent
	loaded, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	clear(body)
	s := loaded.Snapshot()
	clear(s.Payload)
	s.Intent.TopicID = "mutated"
	encoded, _ := loaded.Encode()
	clear(encoded)
	after, _ := loaded.Encode()
	if !bytes.Equal(after, original) || !loaded.Matches(i) {
		t.Fatal("caller mutated record")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				s := loaded.Snapshot()
				if s.Intent != i || len(s.Payload) == 0 {
					t.Error("concurrent snapshot changed")
				}
				clear(s.Payload)
				body, err := loaded.Encode()
				if err != nil || !bytes.Equal(body, original) {
					t.Error("concurrent encoding changed")
				}
				clear(body)
			}
		}()
	}
	wg.Wait()
}
