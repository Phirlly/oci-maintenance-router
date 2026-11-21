package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func prepareApprovedRows(d *recordData, index int, current Revision, values []delivery.Preparation) ([]string, error) {
	ids := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, p := range values {
		s := p.Draft.Snapshot()
		_, exists := d.delivery.Batch(s.BatchID)
		if p.SecretVersion <= 0 || s.BatchID == "" || s.EnvironmentID != d.EnvironmentID || s.Revision.NoticeID != d.NoticeID || s.Revision.Digest != current.Digest || s.RoutingGeneration != current.RoutingGeneration || exists || seen[s.BatchID] {
			return nil, &Error{"invalid_approval_preparation"}
		}
		seen[s.BatchID] = true
		ids = append(ids, s.BatchID)
		for _, member := range s.Resources {
			found := false
			for j, row := range d.Revisions[index].Resources {
				if row.ResourceID != member.ResourceID {
					continue
				}
				if row.BatchID != "" || row.Disposition == "CLOSED" {
					return nil, &Error{"member_not_ready"}
				}
				for _, ready := range current.Resources {
					if ready.ResourceID == member.ResourceID && ready.Region == member.Region && ready.DestinationID == s.DestinationID && ready.Disposition == string(routing.Ready) {
						d.Revisions[index].Resources[j] = ready
						found = true
						break
					}
				}
			}
			if !found {
				return nil, &Error{"member_not_ready"}
			}
		}
	}
	return ids, nil
}

func prepareDeliveries(r delivery.Record, version int, values []delivery.Preparation) (delivery.Record, error) {
	if len(r.Batches()) != 0 {
		if r.SchemaVersion() != deliveryVersion(version) {
			return delivery.Record{}, &Error{"invalid_delivery"}
		}
		return r.Prepare(values...)
	}
	next, err := delivery.NewRecord(values...)
	if err != nil {
		return delivery.Record{}, err
	}
	return adoptDeliveryFormat(next, version)
}
