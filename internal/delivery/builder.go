package delivery

import (
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

// Builder owns validated configuration and is safe for concurrent reads.
// Callers must not mutate lookup maps while Build is reading them.
type Builder struct {
	policy          *routing.Policy
	environmentID   string
	environmentName string
	pagerDutyRegion string
	secrets         map[string]string
}

// New takes trusted catalog and ledger metadata independently of customer JSON.
func New(data []byte, catalog configuration.Catalog, ledger configuration.LedgerLocation) (*Builder, error) {
	c, err := configuration.Decode(data, catalog)
	if err != nil {
		return nil, err
	}
	environmentID, err := c.EnvironmentID(ledger)
	if err != nil {
		return nil, err
	}
	policy, err := routing.New(data, catalog)
	if err != nil {
		return nil, err
	}
	b := &Builder{policy: policy, environmentID: environmentID, environmentName: c.EnvironmentName,
		pagerDutyRegion: c.PagerDutyRegion, secrets: map[string]string{}}
	for _, d := range c.Destinations {
		b.secrets[d.ID] = d.SecretID
	}
	return b, nil
}

// Build prepares only explicitly selected, currently READY resources. The caller
// must exclude existing assignments and enforce source/activation/ledger gates.
// Empty selection means no new work. Never use Build to reconstruct a retry.
func (b *Builder) Build(n notices.Notice, knownCandidate bool, lookups map[string]routing.Lookup, selected []string) (Result, error) {
	if b == nil || b.policy == nil {
		return Result{}, &Error{"builder", "invalid"}
	}
	plan := b.policy.Resolve(n, knownCandidate, lookups)
	if plan.Assessment.State != routing.SupportedShape {
		return Result{}, &Error{"notice", "unsupported"}
	}
	groups, err := selectGroups(plan.Groups, selected)
	if err != nil {
		return Result{}, err
	}
	if len(groups) == 0 {
		return Result{}, nil
	}
	return b.pack(n.Snapshot(), plan, groups), nil
}
