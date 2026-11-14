package configuration

import "encoding/json"

// Select the field contract before allowing version-specific controls.
func configurationVersion(value any) (int, error) {
	fields, ok := value.(map[string]any)
	if !ok {
		return 0, &Error{"configuration", "type"}
	}
	raw, exists := fields["schemaVersion"]
	if !exists {
		return 0, &Error{"schemaVersion", "required"}
	}
	if raw == nil {
		return 0, &Error{"schemaVersion", "type"}
	}
	number, ok := raw.(json.Number)
	if ok {
		switch number {
		case "1":
			return 1, nil
		case "2":
			return 2, nil
		case "3":
			return 3, nil
		}
	}
	return 0, &Error{"schemaVersion", "unsupported"}
}
