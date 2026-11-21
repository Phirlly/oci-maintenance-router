package notices

import "unicode/utf8"

const maxNoticeBytes = 1 << 20
const maxResources = 256

// A cheap lower bound rejects huge strings/collections before projection allocates.
// Exact encoded size, including field names/escaping/context, is checked afterward.
type inputCheck struct {
	remaining int
	err       error
}

func (c *inputCheck) consume(size int) {
	if c.err != nil {
		return
	}
	if size > c.remaining {
		c.err = &Error{"notice", "too_large"}
		return
	}
	c.remaining -= size
}

func (c *inputCheck) items(count, minimumBytes int) {
	if count > c.remaining/minimumBytes {
		c.consume(c.remaining + 1)
		return
	}
	c.consume(count * minimumBytes)
}

type textField struct {
	name     string
	value    *string
	required bool
}

func (c *inputCheck) texts(fields ...textField) {
	for _, field := range fields {
		if c.err != nil {
			return
		}
		if field.value == nil {
			if field.required {
				c.err = &Error{field.name, "required"}
			}
			continue
		}
		c.consume(len(*field.value))
		if c.err == nil && !utf8.ValidString(*field.value) {
			c.err = &Error{field.name, "invalid_utf8"}
		}
	}
}

func validateInput(r Record) error {
	c := inputCheck{remaining: maxNoticeBytes}
	c.texts(
		textField{"noticeId", &r.NoticeID, true}, textField{"announcementType", &r.AnnouncementType, true},
		textField{"chainId", r.ChainID, false}, textField{"reference", r.Reference, false},
		textField{"platform", r.Platform, false}, textField{"summary", r.Summary, false},
		textField{"description", r.Description, false}, textField{"additionalInformation", r.AdditionalInformation, false},
		textField{"timeCreated", r.TimeCreated, false}, textField{"timeUpdated", r.TimeUpdated, false},
		textField{"lifecycleState", r.LifecycleState, false},
	)
	for i, prefix := range []string{"times[0]", "times[1]"} {
		slot := r.Times[i]
		c.texts(textField{prefix + ".type", slot.Type, false},
			textField{prefix + ".title", slot.Title, false}, textField{prefix + ".value", slot.Value, false})
	}
	c.items(len(r.Services), 2) // Even an empty JSON string needs two quotes.
	for _, service := range r.Services {
		if c.err != nil {
			return c.err
		}
		c.texts(textField{"services", &service, true})
	}
	c.items(len(r.Resources), len("{\"resourceId\":\"\",\"resourceName\":\"\",\"regionDisplay\":\"\",\"additionalProperties\":[]}"))
	for _, resource := range r.Resources {
		if c.err != nil {
			return c.err
		}
		c.texts(textField{"resources.resourceId", &resource.ResourceID, true},
			textField{"resources.resourceName", resource.ResourceName, false},
			textField{"resources.regionDisplay", resource.RegionDisplay, false})
		if c.err == nil && resource.ResourceID == "" {
			c.err = &Error{"resources.resourceId", "required"}
		}
		c.items(len(resource.AdditionalProperties), len("{\"name\":\"\",\"value\":\"\"}"))
		for _, property := range resource.AdditionalProperties {
			if c.err != nil {
				return c.err
			}
			c.texts(textField{"resources.additionalProperties.name", property.Name, true},
				textField{"resources.additionalProperties.value", property.Value, false})
		}
	}
	return c.err
}
