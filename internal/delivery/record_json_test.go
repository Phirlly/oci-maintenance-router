package delivery_test

import (
	"bytes"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestDecodeRejectsNoncanonicalAndHostileJSON(t *testing.T) {
	r, _ := prepared(t, db)
	s := r.Batches()[0].Message
	valid := encoded(t, r)
	for name, data := range map[string][]byte{
		"empty": nil, "null": []byte("null"), "syntax": []byte("{"),
		"object-key": []byte(`{false:1}`), "wrong-close": []byte(`{"batches":[}`),
		"missing-close": []byte(`{"batches":[null`), "array-close": []byte(`]`),
		"whitespace": append([]byte(" "), valid...), "trailing": append(append([]byte{}, valid...), []byte("{}")...),
		"unknown":          bytes.Replace(valid, []byte(`"schemaVersion":1`), []byte(`"private-field":1,"schemaVersion":1`), 1),
		"duplicate":        bytes.Replace(valid, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1),
		"case":             bytes.Replace(valid, []byte(`"schemaVersion"`), []byte(`"SchemaVersion"`), 1),
		"missing":          bytes.Replace(valid, []byte(`,"ambiguous":false`), nil, 1),
		"null-count":       bytes.Replace(valid, []byte(`"attemptCount":0`), []byte(`"attemptCount":null`), 1),
		"alternate-number": bytes.Replace(valid, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1.0`), 1),
		"surrogate":        bytes.Replace(valid, []byte(`"ownerToken":""`), []byte(`"ownerToken":"\ud800"`), 1),
		"utf8":             append([]byte{0xff}, valid...), "size": []byte(strings.Repeat(" ", 1<<20+1)),
		"depth":      []byte(strings.Repeat("[", 33) + strings.Repeat("]", 33)),
		"allocation": []byte(`{"batches":[` + strings.Repeat("null,", 8192) + `null]}`),
		"wrong-type": bytes.Replace(valid, []byte(`"secretVersion":7`), []byte(`"secretVersion":"7"`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := delivery.DecodeRecord(data, s.EnvironmentID, s.Revision.NoticeID)
			rejectedRecord(t, got, err)
		})
	}
}

func TestDecodeRejectsSavedMessageTampering(t *testing.T) {
	r, _ := prepared(t, db)
	s := r.Batches()[0].Message
	valid := encoded(t, r)
	for _, replacement := range [][2]string{
		{`"schemaVersion":1`, `"schemaVersion":2`}, {`"secretVersion":7`, `"secretVersion":0`},
		{`"destinationId":"payments"`, `"destinationId":"other"`}, {`"pagerDutyRegion":"US"`, `"pagerDutyRegion":"invalid"`},
		{`"secretId":"ocid1.vaultsecret.oc1.iad.payments"`, `"secretId":"private-key"`},
		{`"routingGeneration":"` + s.RoutingGeneration + `"`, `"routingGeneration":"invalid"`},
		{`"payloadDigest":"` + s.PayloadDigest + `"`, `"payloadDigest":"` + strings.Repeat("0", 64) + `"`},
		{`"batchId":"` + s.BatchID + `"`, `"batchId":"` + strings.Repeat("0", 64) + `"`},
		{`"revisionDigest":"` + s.Revision.Digest + `"`, `"revisionDigest":"invalid"`},
		{`"resourceId":"` + db + `"`, `"resourceId":"ocid1.instance.oc1.iad.other"`},
		{`"region":"us-ashburn-1"`, `"region":"invalid region"`},
		{`"dedupKey":"omr1:`, `"dedupKey":"other:`},
		{`"state":"PREPARED"`, `"state":"API_ACCEPTED"`},
	} {
		data := bytes.Replace(valid, []byte(replacement[0]), []byte(replacement[1]), 1)
		if bytes.Equal(data, valid) {
			t.Fatalf("fixture missing %s", replacement[0])
		}
		got, err := delivery.DecodeRecord(data, s.EnvironmentID, s.Revision.NoticeID)
		rejectedRecord(t, got, err)
	}
}
