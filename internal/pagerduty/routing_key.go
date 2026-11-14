package pagerduty

import "unicode/utf8"

// ValidRoutingKey checks the existing wire shape before immutable version pinning.
// It does not verify that PagerDuty recognizes the key or authorize delivery.
func ValidRoutingKey(key []byte) bool {
	return len(key) >= 32 && len(key) <= 128 && utf8.Valid(key) && utf8.RuneCount(key) == 32
}
