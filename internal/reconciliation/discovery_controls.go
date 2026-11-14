package reconciliation

func (r *discoveryRun) reserve() error {
	return currentReserve(r.ctx, r.lease)
}
func (r *discoveryRun) guard() error {
	return r.owner.controls().check(r.ctx, r.lease)
}
