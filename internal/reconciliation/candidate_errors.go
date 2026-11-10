package reconciliation

const (
	InvalidNotice       Code = "invalid_notice"
	InvalidRecord       Code = "invalid_record"
	EnvironmentMismatch Code = "environment_mismatch"
)

type CandidateError struct{ Code Code }

func (e *CandidateError) Error() string { return "candidate relevance: " + string(e.Code) }

func (c *Candidates) valid() error {
	if c == nil || c.policy == nil || c.environmentID == "" {
		return &CandidateError{InvalidRequest}
	}
	if c.failed != "" {
		return &CandidateError{c.failed}
	}
	return nil
}

func (c *Candidates) fail(code Code) error {
	c.failed = code
	return &CandidateError{code}
}
