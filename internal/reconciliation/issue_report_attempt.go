package reconciliation

import "time"

const reportCooldown = 5 * time.Minute

func claimReport(s *reportState, token string, now, deadline time.Time) (bool, error) {
	if !leaseDigest.MatchString(token) || !leaseWallTime(now) || !leaseWallTime(deadline) || !leaseWallTime(now.Add(reportCooldown)) || !deadline.After(now) {
		return false, &ReportingError{"invalid_attempt"}
	}
	if s.Pending == nil {
		return false, nil
	}
	if s.NotBefore != "" {
		next, _ := leaseTimestamp(s.NotBefore)
		if now.Before(next) {
			return false, nil
		}
	}
	p := *s.Pending
	if p.Claim == token {
		return false, &ReportingError{"claim_reused"}
	}
	candidate := *s
	candidate.NotBefore = now.Add(reportCooldown).UTC().Truncate(time.Millisecond).Format(leaseTimeFormat)
	p.Claim, p.IssuedAt, p.Deadline, p.Status = token, now.UTC().Truncate(time.Millisecond).Format(leaseTimeFormat), deadline.UTC().Truncate(time.Millisecond).Format(leaseTimeFormat), 0
	candidate.Pending = &p
	if _, err := encodeReport(candidate); err != nil {
		return false, err
	}
	*s = candidate
	return true, nil
}

func finishReport(s *reportState, token string, status int, messageID string, at time.Time) error {
	if s.Pending == nil || s.Pending.Claim != token || !leaseDigest.MatchString(token) || !leaseWallTime(at) {
		return &ReportingError{"stale_outcome"}
	}
	issued, _ := leaseTimestamp(s.Pending.IssuedAt)
	deadline, _ := leaseTimestamp(s.Pending.Deadline)
	if at.Before(issued) || !at.Before(deadline) || status != 0 && (status < 100 || status > 599) {
		return &ReportingError{"invalid_outcome"}
	}
	candidate := *s
	p := *s.Pending
	p.Status = status
	candidate.Pending = &p
	if (status == 200 || status == 202) && operatorOpaqueText(messageID, 256) {
		candidate.Accepted, candidate.AcceptedHash, candidate.AcceptedID, candidate.AcceptedStatus = p.Revision, p.Digest, messageID, status
		candidate.AcceptedAt = at.UTC().Truncate(time.Millisecond).Format(leaseTimeFormat)
		candidate.Pending = nil
	}
	if _, err := encodeReport(candidate); err != nil {
		return err
	}
	*s = candidate
	return nil
}
