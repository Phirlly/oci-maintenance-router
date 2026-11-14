package ocistorage

import "strings"

func decodeHistoryEmpty(data []byte, name string) (bool, error) {
	value, err := listingJSON(data)
	if err != nil {
		return false, err
	}
	object, ok := value.(map[string]any)
	if !ok || listingAliases(object, "items", "prefixes") {
		return false, &Error{Code: InvalidResponse}
	}
	items, ok := object["items"].([]any)
	if !ok {
		return false, &Error{Code: InvalidResponse}
	}
	if len(items) > 1 {
		return false, &Error{Code: TooLarge}
	}
	if prefixes := object["prefixes"]; prefixes != nil {
		list, ok := prefixes.([]any)
		if !ok || len(list) != 0 {
			return false, &Error{Code: InvalidResponse}
		}
	}
	for _, item := range items {
		summary, ok := item.(map[string]any)
		if !ok || listingAliases(summary, "name") {
			return false, &Error{Code: InvalidResponse}
		}
		found, ok := summary["name"].(string)
		if !ok || !safeName(found) || !strings.HasPrefix(found, name) {
			return false, &Error{Code: InvalidResponse}
		}
	}
	// Any matching prefix entry blocks initialization. We do not interpret its
	// version metadata or claim that a longer name proves exact-key history.
	return len(items) == 0, nil
}
