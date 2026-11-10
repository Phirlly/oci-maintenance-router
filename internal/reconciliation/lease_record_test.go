package reconciliation

import (
	"strings"
	"testing"
	"time"
)

func TestLeaseMalformedOrForeignRecordCannotBeTakenOver(t *testing.T) {
	now := leaseTime()
	env, owner := strings.Repeat("a", 64), strings.Repeat("b", 64)
	valid := string(leaseBody(env, owner, now.Add(-600*time.Second), now))
	cases := map[string]string{
		"empty": "", "null": "null", "missing fields": "{}", "array": "[]",
		"noncanonical":        valid + " ",
		"schema":              strings.Replace(valid, "\"schemaVersion\":1", "\"schemaVersion\":2", 1),
		"foreign environment": strings.Replace(valid, env, strings.Repeat("c", 64), 1),
		"owner":               strings.Replace(valid, owner, "bad", 1),
		"oversized":           strings.Repeat(" ", 4097),
		"short duration":      string(leaseBody(env, owner, now, now.Add(time.Second))),
		"negative duration":   string(leaseBody(env, owner, now, now.Add(-time.Second))),
		"unknown field":       strings.Replace(valid, "\"schemaVersion\":1", "\"extra\":0,\"schemaVersion\":1", 1),
		"duplicate field":     strings.Replace(valid, "\"schemaVersion\":1", "\"schemaVersion\":1,\"schemaVersion\":1", 1),
		"field case":          strings.Replace(valid, "acquiredAt", "AcquiredAt", 1),
		"invalid year":        strings.Replace(valid, now.Format("2006-01-02T15:04:05.000Z"), "0000-01-01T00:00:00.000Z", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			s := &leaseMemory{body: []byte(body), version: 1}
			l, e := leaseManager(t, s, now).Acquire(leaseBudget(t))
			leaseFailure(t, e, "invalid_record")
			if l != nil || s.version != 1 {
				t.Fatal("invalid record overwritten")
			}
		})
	}
}
