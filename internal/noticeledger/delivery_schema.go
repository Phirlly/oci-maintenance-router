package noticeledger

import "oci-maintenance-router/internal/delivery"

func deliveryVersion(version int) int {
	switch version {
	case 1, 2:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	}
	return 0
}

// Only a newly prepared record adopts the enclosing format. Existing records
// must already match it; explicit enclosing migration owns their upgrades.
func adoptDeliveryFormat(r delivery.Record, version int) (delivery.Record, error) {
	want := deliveryVersion(version)
	if want == 0 || r.SchemaVersion() == 0 || r.SchemaVersion() > want {
		return delivery.Record{}, &Error{"invalid_delivery"}
	}
	var err error
	if r.SchemaVersion() == 1 && want >= 2 {
		r, err = r.MigrateRecovery()
		if err != nil {
			return delivery.Record{}, err
		}
	}
	if r.SchemaVersion() == 2 && want == 3 {
		return r.MigrateAnnouncements()
	}
	return r, nil
}
