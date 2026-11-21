package noticeledger

import (
	"bytes"
	"os"
	"testing"
)

func recoveryGolden(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/source-only-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

func TestRecoveryCodecIndependentGoldenAndLegacyBytes(t *testing.T) {
	const env = "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"
	want := recoveryGolden(t)
	r, err := Decode(want, env, noticeID)
	if err != nil || r.Snapshot().SchemaVersion != 3 || r.Snapshot().Recovery == nil {
		t.Fatal("v3 golden rejected", err)
	}
	if !bytes.Equal(want, encoded(t, r)) {
		t.Fatal("restart rewrote v3")
	}
	for _, file := range []string{"source-only.json", "source-only-v2.json"} {
		body, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.TrimSpace(body)
		old, err := Decode(body, env, noticeID)
		if err != nil || !bytes.Equal(body, encoded(t, old)) || old.Snapshot().Recovery != nil {
			t.Fatal("legacy bytes or schema changed", err)
		}
		if old.Snapshot().SchemaVersion == 2 && !bytes.Equal(encoded(t, recoveryRecord(t, old)), want) {
			t.Fatal("migration differs from wire specification")
		}
	}
}

func TestRecoveryCodecRejectsWrongVersionAndPresence(t *testing.T) {
	const env = "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378"
	golden := recoveryGolden(t)
	for _, change := range [][2]string{
		{`"schemaVersion":3`, `"schemaVersion":2`},
		{`"schemaVersion":3`, `"schemaVersion":5`},
		{`,"recovery":{"actions":[]}`, ``},
		{`"recovery":{"actions":[]}`, `"recovery":null`},
		{`"actions":[]`, `"actions":null`},
		{`"actions":[]`, `"actions":[],"extra":true`},
		{`"actions":[]`, `"Actions":[]`},
		{`"actions":[]`, `"actions":[],"actions":[]`},
	} {
		body := bytes.Replace(golden, []byte(change[0]), []byte(change[1]), 1)
		if bytes.Equal(body, golden) {
			t.Fatal("fixture unchanged")
		}
		r, err := Decode(body, env, noticeID)
		rejected(t, r, err)
	}
}

func TestRecoveryCodecRequiresNestedRecoveryVersion(t *testing.T) {
	p := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	body := changedWire(t, p, func(w *wireRecord) { w.SchemaVersion = 3; w.Recovery = &recoveryData{Actions: []RecoveryAction{}} })
	env, id := p.Identity()
	r, err := Decode(body, env, id)
	rejected(t, r, err)
}

func TestRecoveryCodecIndependentRetainedActionGolden(t *testing.T) {
	body, err := os.ReadFile("testdata/rejected-recovery-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	r := recoveryRecord(t, observed(t, source(db)))
	next, _, err := r.RejectRecovery(recoveryIntent(t), "readiness_required", clock())
	if err != nil || !bytes.Equal(bytes.TrimSpace(body), encoded(t, next)) {
		t.Fatal("retained action differs from independent wire specification", err)
	}
	env, id := next.Identity()
	decoded, err := Decode(bytes.TrimSpace(body), env, id)
	if err != nil || !bytes.Equal(encoded(t, decoded), bytes.TrimSpace(body)) {
		t.Fatal("independent action fixture rejected", err)
	}
}
