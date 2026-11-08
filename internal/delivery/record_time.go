package delivery

import "time"

const recordTimeFormat = "2006-01-02T15:04:05.000Z"

func operationTime(value time.Time, previous *string) (time.Time, error) {
	v := value.UTC().Truncate(time.Millisecond)
	if v.IsZero() || v.Year() < 1 || v.Year() > 9999 {
		return time.Time{}, &Error{"time", "invalid"}
	}
	if previous != nil && v.Before(savedTime(previous)) {
		return time.Time{}, &Error{"time", "retrograde"}
	}
	return v, nil
}

func timeText(value time.Time) *string { text := value.Format(recordTimeFormat); return &text }

// Only validated private values reach transitions and snapshot access.
func savedTime(value *string) time.Time {
	if value == nil {
		return time.Time{}
	}
	t, _ := time.Parse(recordTimeFormat, *value)
	return t
}
