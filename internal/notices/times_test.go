package notices

import "testing"

func TestTimestampNormalization(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"2026-01-16T01:30:00+01:30", "2026-01-16T00:00:00.000Z"},
		{"2026-01-15T18:00:00-06:00", "2026-01-16T00:00:00.000Z"},
		{"2026-01-16T00:00:00.999999999Z", "2026-01-16T00:00:00.999Z"},
		{"2026-01-16T00:00:00.1Z", "2026-01-16T00:00:00.100Z"},
		{"0000-01-01T00:00:00Z", "0000-01-01T00:00:00.000Z"},
		{"9999-12-31T23:59:59Z", "9999-12-31T23:59:59.000Z"},
	} {
		input := sample()
		input.Times[0].Value = text(tc.input)
		got := normalized(t, input).Snapshot()
		if *got.Times[0].Value != tc.want {
			t.Fatalf("%q normalized to %q, want %q", tc.input, *got.Times[0].Value, tc.want)
		}
	}
}

func TestRejectInvalidTimestamps(t *testing.T) {
	for _, value := range []string{
		"", "2026-01-16", "2026-01-16T00:00:00", "2026-01-16 00:00:00Z",
		"2026-01-16T0:00:00Z", "2026-01-16T00:00:00,123Z",
		"2026-01-16T00:00:00.1234567890Z", "2026-01-16T00:00:00z",
		"2026-01-16T00:00:00+24:00", "2026-01-16T00:00:00+00:60",
		"2026-01-16T00:00:00-00:00", "2026-01-16T24:00:00Z",
		"2026-01-16T00:60:00Z", "2026-01-16T00:00:60Z",
		"2026-02-29T00:00:00Z", "2026-13-01T00:00:00Z",
		"0000-01-01T00:00:00+01:00", "9999-12-31T23:59:59-01:00",
	} {
		t.Run(value, func(t *testing.T) {
			input := sample()
			input.Times[0].Value = text(value)
			wantError(t, input, "times[0].value", "invalid_timestamp")
		})
	}
	for _, field := range []string{"times[1].value", "timeCreated", "timeUpdated"} {
		input := sample()
		switch field {
		case "times[1].value":
			input.Times[1].Value = text("sensitive-value")
		case "timeCreated":
			input.TimeCreated = text("sensitive-value")
		case "timeUpdated":
			input.TimeUpdated = text("sensitive-value")
		}
		wantError(t, input, field, "invalid_timestamp")
	}
}
