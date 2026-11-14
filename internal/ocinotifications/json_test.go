package ocinotifications

import (
	"strings"
	"testing"
)

func TestWireJSONRejectsAmbiguityAndMalformedUnicode(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	for _, extra := range []string{`,"topicId":"duplicate"`, `,"\u0074opicId":"escaped"`, `,"future":{"x":0,"x":1}`, `,"future":"\ud800"`, `,"future":"\udfff"`, `,"future":"\ud800\u0041"`, `,"future":"\ud800x"`} {
		ch, err := c.decodeTopic([]byte(strings.TrimSuffix(topicJSON, "}")+extra+"}"), topicID)
		wantEmpty(t, ch)
		if err == nil {
			t.Fatal("ambiguous JSON admitted")
		}
	}
	for _, body := range [][]byte{nil, []byte("{"), []byte(topicJSON + "{}"), []byte(strings.Replace(topicJSON, "example", string([]byte{255}), 1))} {
		ch, err := c.decodeTopic(body, topicID)
		wantEmpty(t, ch)
		if err == nil {
			t.Fatal("invalid document admitted")
		}
	}
}
func TestWireJSONRetainsAdditiveCompatibilityWithinBounds(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	for _, value := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"\u0041\n\t\""`, `1e999`, strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31)} {
		if _, err := c.decodeTopic([]byte(strings.TrimSuffix(topicJSON, "}")+`,"future":`+value+"}"), topicID); err != nil {
			t.Fatal("valid unknown metadata rejected")
		}
	}
	for _, extra := range []string{`,"future":"` + strings.Repeat("x", bodyLimit) + `"`, `,"future":` + strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32), `,"future":[` + strings.Repeat("0,", valueLimit) + "0]"} {
		ch, err := c.decodeTopic([]byte(strings.TrimSuffix(topicJSON, "}")+extra+"}"), topicID)
		wantEmpty(t, ch)
		wantError(t, err, TooLarge, 0, false)
	}
}
