package pagerduty

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retry-After is one date or decimal seconds, not a comma-separated list.
// Ignore malformed/duplicate values; never turn a valid huge delay into zero.
func retryAfter(header http.Header, received time.Time) (time.Time, bool) {
	values := header.Values("Retry-After")
	if len(values) != 1 {
		return time.Time{}, false
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return time.Time{}, false
	}
	digits := true
	for _, c := range value {
		if c < '0' || c > '9' {
			digits = false
			break
		}
	}
	if !digits {
		at, err := http.ParseTime(value)
		if err != nil || !at.After(received) {
			return time.Time{}, false
		}
		return at.UTC(), false
	}
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return received, false
	}
	seconds, err := strconv.ParseUint(value, 10, 64)
	// Construct seconds directly rather than overflowing time.Duration (~292y).
	last := time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
	if err != nil || seconds > uint64(last.Unix()-received.Unix()) {
		return time.Time{}, true
	}
	return time.Unix(received.Unix()+int64(seconds), int64(received.Nanosecond())).UTC(), false
}
