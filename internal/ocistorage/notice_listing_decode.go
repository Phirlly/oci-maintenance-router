package ocistorage

import "strings"

func decodeNoticePage(data []byte) (NoticePage, error) {
	value, err := listingJSON(data)
	if err != nil {
		return NoticePage{}, err
	}
	object, ok := value.(map[string]any)
	if !ok || listingAliases(object, "objects", "nextStartWith", "prefixes") {
		return NoticePage{}, &Error{Code: InvalidResponse}
	}
	items, ok := object["objects"].([]any)
	if !ok {
		return NoticePage{}, &Error{Code: InvalidResponse}
	}
	if len(items) > 1000 {
		return NoticePage{}, &Error{Code: TooLarge}
	}
	if prefixes := object["prefixes"]; prefixes != nil {
		list, ok := prefixes.([]any)
		if !ok || len(list) != 0 {
			return NoticePage{}, &Error{Code: InvalidResponse}
		}
	}
	page := NoticePage{Names: make([]string, 0, len(items))}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		summary, ok := item.(map[string]any)
		if !ok || listingAliases(summary, "name") {
			return NoticePage{}, &Error{Code: InvalidResponse}
		}
		name, ok := summary["name"].(string)
		if !ok || !validNoticeObjectName(name) || seen[name] {
			return NoticePage{}, &Error{Code: InvalidResponse}
		}
		seen[name] = true
		page.Names = append(page.Names, name)
	}
	if next := object["nextStartWith"]; next != nil {
		cursor, ok := next.(string)
		if !ok || (cursor != "" && !validNoticeObjectName(cursor)) {
			return NoticePage{}, &Error{Code: InvalidResponse}
		}
		page.NextStartWith = cursor
	}
	return page, nil
}

func listingAliases(object map[string]any, names ...string) bool {
	for key := range object {
		for _, name := range names {
			if key != name && strings.EqualFold(key, name) {
				return true
			}
		}
	}
	return false
}
