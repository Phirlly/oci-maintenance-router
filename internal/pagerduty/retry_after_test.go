package pagerduty

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 500000000, time.UTC)
	for _, tc := range []struct {
		name     string
		values   []string
		want     time.Time
		overflow bool
	}{
		{name: "absent"}, {name: "empty", values: []string{""}},
		{name: "seconds", values: []string{"120"}, want: now.Add(120 * time.Second)},
		{name: "zero", values: []string{"0"}, want: now},
		{name: "leading zeros", values: []string{"000120"}, want: now.Add(120 * time.Second)},
		{name: "padding", values: []string{" \t120\t "}, want: now.Add(120 * time.Second)},
		{name: "date", values: []string{"Mon, 09 Nov 2026 12:02:00 GMT"}, want: time.Date(2026, 11, 9, 12, 2, 0, 0, time.UTC)},
		{name: "past", values: []string{"Mon, 09 Nov 2026 11:00:00 GMT"}},
		{name: "invalid date", values: []string{"Wed, 99 Nov 2026 12:02:00 GMT"}},
		{name: "negative", values: []string{"-1"}}, {name: "plus", values: []string{"+1"}},
		{name: "fraction", values: []string{"1.5"}}, {name: "combined", values: []string{"10, 20"}},
		{name: "duplicates", values: []string{"10", "20"}},
		{name: "too distant", values: []string{"999999999999"}, overflow: true},
		{name: "integer overflow", values: []string{strings.Repeat("9", 1024)}, overflow: true},
		{name: "many zeros", values: []string{strings.Repeat("0", 1024) + "1"}, want: now.Add(time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, over := retryAfter(http.Header{"Retry-After": tc.values}, now)
			if !got.Equal(tc.want) || over != tc.overflow {
				t.Fatalf("delay=%v overflow=%t", got, over)
			}
		})
	}
}
