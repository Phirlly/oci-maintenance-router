package configuration

import (
	"strings"
	"testing"
)

func TestModesAndBaseline(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "LIVE"} {
		all := settings(t)
		all["mode"] = mode
		all["activeBaselineId"] = "baseline_01-A"
		got := decoded(t, all)
		if got.Mode != mode || got.ActiveBaselineID != "baseline_01-A" {
			t.Fatalf("incorrect controls: %+v", got)
		}
	}
	for field, values := range map[string][]any{
		"mode":             {"live", "", nil, 1},
		"activeBaselineId": {"", "../baseline", strings.Repeat("a", 129), 1},
		"replay":           {nil, true},
	} {
		for _, value := range values {
			all := settings(t)
			all[field] = value
			if _, err := Decode(encoded(t, all), catalog()); err == nil {
				t.Fatalf("accepted %s=%v", field, value)
			}
		}
	}
	all := settings(t)
	all["mode"] = "LIVE"
	wantError(t, encoded(t, all), "activeBaselineId", "required_for_live")
}

func TestReplaySelection(t *testing.T) {
	digest := strings.Repeat("a", 64)
	selection := map[string]any{"noticeId": "ocid1.announcement.oc1..example", "revisionDigest": digest}
	all := settings(t)
	all["replay"] = []any{selection}
	got := decoded(t, all)
	if len(got.Replay) != 1 || got.Replay[0] != (Replay{"ocid1.announcement.oc1..example", digest}) {
		t.Fatal("selection changed")
	}
	for _, invalid := range []any{
		[]any{nil}, []any{selection, selection},
		[]any{map[string]any{"noticeId": "ocid1.instance.oc1.iad.example", "revisionDigest": digest}},
		[]any{map[string]any{"noticeId": "ocid1.announcement.oc1..example", "revisionDigest": strings.Repeat("A", 64)}},
		[]any{map[string]any{"noticeId": "ocid1.announcement.oc1..example", "revisionDigest": "abc"}},
		[]any{map[string]any{"noticeId": "ocid1.announcement.oc1..example", "revisionDigest": digest, "payload": "arbitrary"}},
	} {
		all["replay"] = invalid
		if _, err := Decode(encoded(t, all), catalog()); err == nil {
			t.Fatalf("accepted replay %#v", invalid)
		}
	}
}
