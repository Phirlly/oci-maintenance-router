package configuration

// validateLiveInputs requires explicit receipt inputs, not proof of readiness.
// The activation coordinator must verify both saved receipts, current channel,
// cumulative baseline, current configuration and real lease independently.
func (c Config) validateLiveInputs() error {
	if c.Mode != "LIVE" {
		return nil
	}
	if c.Diagnostic == nil {
		return &Error{"diagnostic", "required_for_live"}
	}
	if c.Diagnostic.ReceiptCode == "" {
		return &Error{"diagnostic.receiptCode", "required_for_live"}
	}
	if c.OperatorTest == nil {
		return &Error{"operatorTest", "required_for_live"}
	}
	if c.OperatorTest.ReceiptCode == "" {
		return &Error{"operatorTest.receiptCode", "required_for_live"}
	}
	return nil
}
