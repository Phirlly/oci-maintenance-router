package noticeledger

import "time"

const timeFormat = "2006-01-02T15:04:05.000Z"

func (r Record) operationTime(at time.Time) (string, error) {
	v := at.UTC().Truncate(time.Millisecond)
	if v.IsZero() || v.Year() < 1 || v.Year() > 9999 {
		return "", &Error{"invalid_time"}
	}
	stamp := v.Format(timeFormat)
	if r.data != nil && stamp < r.data.UpdatedAt {
		return "", &Error{"retrograde_time"}
	}
	return stamp, nil
}
