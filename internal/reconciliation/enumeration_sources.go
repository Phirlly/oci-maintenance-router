package reconciliation

import "strings"

func (e *enumeration) announcements(source AnnouncementPages) ([]string, error) {
	var result []string
	cursor, seen := "", map[string]bool{}
	for count := 0; count < enumerationPages; count++ {
		if err := e.before(Announcements); err != nil {
			return nil, err
		}
		page, err := source.ListPage(e.ctx, cursor)
		if err != nil {
			return nil, readFailure(e.ctx, Announcements, err)
		}
		if err = e.accept(Announcements, seen, page.IDs, page.NextPage, 100, 4096); err != nil {
			return nil, err
		}
		result = appendIdentifiers(result, page.IDs)
		if page.NextPage == "" {
			return result, nil
		}
		cursor = strings.Clone(page.NextPage)
	}
	return nil, failure(e.ctx, Announcements, PageLimit)
}

func (e *enumeration) savedNotices(source SavedNoticePages) ([]string, error) {
	var result []string
	cursor, seen := "", map[string]bool{}
	for count := 0; count < enumerationPages; count++ {
		if err := e.before(SavedNotices); err != nil {
			return nil, err
		}
		page, err := source.ListPage(e.ctx, cursor)
		if err != nil {
			return nil, readFailure(e.ctx, SavedNotices, err)
		}
		if err = e.accept(SavedNotices, seen, page.Names, page.NextStartWith, 1000, 77); err != nil {
			return nil, err
		}
		result = appendIdentifiers(result, page.Names)
		if page.NextStartWith == "" {
			return result, nil
		}
		cursor = strings.Clone(page.NextStartWith)
	}
	return nil, failure(e.ctx, SavedNotices, PageLimit)
}
