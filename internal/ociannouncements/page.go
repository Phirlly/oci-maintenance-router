package ociannouncements

import "net/http"

const pageLimit = 100

// Page preserves source order and duplicates for scan accounting. An empty page
// with NextPage is not terminal. Callers must fetch every detail before classifying
// it, follow all pages and detect cycles; this adapter grants no scan authority.
type Page struct {
	IDs      []string
	NextPage string
}

func decodePage(data []byte, header http.Header, previous string) (Page, error) {
	value, err := strictJSON(data)
	if err != nil {
		return Page{}, err
	}
	valid := true
	f := objectFields(value, &valid, "items")
	items := f.array("items", true)
	if len(items) > pageLimit {
		return Page{}, &Error{Code: TooLarge}
	}
	page := Page{}
	for _, item := range items {
		entry := objectFields(item, &valid, "id")
		id := entry.nonempty("id")
		if !validID(id, announcementID) {
			valid = false
		}
		page.IDs = append(page.IDs, id)
	}
	if values, present := header["Opc-Next-Page"]; present {
		if len(values) != 1 || values[0] == "" || !validPageToken(values[0]) || values[0] == previous {
			valid = false
		} else {
			page.NextPage = values[0]
		}
	}
	if !valid {
		return Page{}, &Error{Code: InvalidResponse}
	}
	return page, nil
}
