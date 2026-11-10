package noticeledger

import "testing"

func TestCaptureSizeCountsEscapesUTF8AndCompleteResourceRows(t *testing.T) {
	plain := source(db)
	plain.Reference = text("x")
	special := source(db)
	special.Reference = text("<>&\n🐬")
	a, b := captured(t, normalized(t, plain)), captured(t, normalized(t, special))
	// JSON: three six-byte HTML escapes, two-byte newline escape, four-byte
	// emoji and two quotes =26 bytes, versus3 bytes for the quoted x.
	if b.RetainedBytes()-a.RetainedBytes() != 23 {
		t.Fatal("wrong escaped UTF8 accounting")
	}
	one, two := captured(t, normalized(t, source(db))), captured(t, normalized(t, source(db, otherDB)))
	row := `{"index":1,"resourceId":"ocid1.mysqldbsystem.oc1.iad.other","region":"us-ashburn-1","disposition":"UNRESOLVED","reason":"lookup_missing","destinationId":"","batchId":""}`
	if two.RetainedBytes()-one.RetainedBytes() != len(row)+1 {
		t.Fatal("resource row or separator omitted")
	}
}
