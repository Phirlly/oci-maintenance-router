package configuration

// RecoveryRequest is desired intent for one exact saved notice target. It does
// not establish current prerequisites, consume an action or authorize dispatch.
type RecoveryRequest struct {
	RequestID, NoticeID, Kind, TargetID                        string
	ReviewFingerprint, RoutingGeneration, ComponentFingerprint string
}

func (c *Config) readRecovery(r objectReader) {
	value, exists := r.values["recovery"]
	if !exists {
		return
	}
	q := readObject(value, "recovery", r.err, []string{"requestId", "noticeId", "kind", "targetId", "reviewFingerprint", "routingGeneration"}, []string{"componentFingerprint"})
	c.Recovery = &RecoveryRequest{RequestID: q.text("requestId"), NoticeID: q.text("noticeId"), Kind: q.text("kind"),
		TargetID: q.text("targetId"), ReviewFingerprint: q.text("reviewFingerprint"), RoutingGeneration: q.text("routingGeneration")}
	_, component := q.values["componentFingerprint"]
	switch c.Recovery.Kind {
	case "CLOSE_BATCH", "CLOSE_REVISION":
		if component {
			q.fail("recovery.componentFingerprint", "forbidden")
		}
	case "RETRY_BATCH", "APPROVE_REVISION":
		if !component {
			q.fail("recovery.componentFingerprint", "required")
		}
	}
	if component && c.Recovery.Kind != "CLOSE_BATCH" && c.Recovery.Kind != "CLOSE_REVISION" {
		c.Recovery.ComponentFingerprint = q.text("componentFingerprint")
	}
}

func (c Config) validateRecovery() error {
	q := c.Recovery
	if q == nil {
		return nil
	}
	if !baselinePattern.MatchString(q.RequestID) {
		return &Error{"recovery.requestId", "invalid"}
	}
	if len(q.NoticeID) > 4096 || !globalOCID(q.NoticeID, "announcement") {
		return &Error{"recovery.noticeId", "invalid"}
	}
	switch q.Kind {
	case "RETRY_BATCH", "APPROVE_REVISION":
		if !digestPattern.MatchString(q.ComponentFingerprint) {
			return &Error{"recovery.componentFingerprint", "invalid"}
		}
	case "CLOSE_BATCH", "CLOSE_REVISION":
	default:
		return &Error{"recovery.kind", "invalid"}
	}
	for _, field := range []struct{ name, value string }{
		{"targetId", q.TargetID}, {"reviewFingerprint", q.ReviewFingerprint}, {"routingGeneration", q.RoutingGeneration},
	} {
		if !digestPattern.MatchString(field.value) {
			return &Error{"recovery." + field.name, "invalid"}
		}
	}
	return nil
}
