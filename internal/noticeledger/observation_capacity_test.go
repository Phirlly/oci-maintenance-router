package noticeledger

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCapturedApplyAdmitsCompleteRecordWithoutChangingPriorHistory(t *testing.T) {
	src := source(db)
	r := observed(t, src)
	for i := 0; i < 31; i++ {
		src.Reference = text(fmt.Sprintf("%02d", i) + strings.Repeat("x", 30000))
		r = applyCaptured(t, captured(t, normalized(t, src)), r)
	}
	before := encoded(t, r)
	src.Reference = text("tail")
	trial := applyCaptured(t, captured(t, normalized(t, src)), r)
	room := (1 << 20) - maximumSize(t, trial)
	if room < 0 || room > 100000 {
		t.Fatal("invalid capacity fixture")
	}
	src.Reference = text("tail" + strings.Repeat("x", room))
	full := applyCaptured(t, captured(t, normalized(t, src)), r)
	if maximumSize(t, full) != 1<<20 {
		t.Fatal("complete record boundary changed")
	}
	reload(t, full)
	src.Reference = text(*src.Reference + "x")
	capture := captured(t, normalized(t, src))
	next, e := capture.Apply(r, clock())
	rejected(t, next, e)
	if !bytes.Equal(before, encoded(t, r)) {
		t.Fatal("failed admission changed prior history")
	}
	// The source remains a valid observation for another record; capture does not
	// perform complete-record admission or discard a potentially relevant source.
	applyCaptured(t, capture, Record{})
}
