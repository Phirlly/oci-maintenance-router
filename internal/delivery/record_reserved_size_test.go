package delivery

import "testing"

func TestReservedBytesExposesActualTransitionHeadroom(t *testing.T) {
	r, _ := codecFixture(t, "escaped<>&🐬")
	got, err := r.ReservedBytes()
	if err != nil || got != maximumMetadata(r.copy()) {
		t.Fatalf("reserved size=%d error=%v", got, err)
	}
	data, _ := r.Encode()
	if got <= len(data) {
		t.Fatal("future transition metadata not reserved")
	}
	if _, err := (Record{}).ReservedBytes(); err == nil {
		t.Fatal("zero record has a budget")
	}
}
