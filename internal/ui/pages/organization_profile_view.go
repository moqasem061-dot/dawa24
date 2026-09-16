package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Building the organization-profile view.
//
// The section list is derived from org.ProfileSections() rather than written
// out here, so a section added to the domain shows up on the page instead of
// being silently missing from it.

// BuildOrganizationSections turns stored values and open requests into the
// page's sections, in the domain's own order.
func BuildOrganizationSections(
	lang string,
	stored map[org.ProfileSection]org.ProfileFields,
	pending map[org.ProfileSection]*org.ProfileChangeRequest,
) []OrganizationProfileSection {
	sections := org.ProfileSections()
	out := make([]OrganizationProfileSection, 0, len(sections))
	for _, key := range sections {
		fields := stored[key]
		if fields == nil {
			fields = org.ProfileFields{}
		}
		out = append(out, OrganizationProfileSection{
			Key:      key,
			Title:    i18n.Translate(lang, fmt.Sprintf("org.profile.section.%s", key)),
			Subtitle: i18n.Translate(lang, fmt.Sprintf("org.profile.section.%s_sub", key)),
			Fields:   fields,
			Pending:  pending[key],
		})
	}
	return out
}

// organizationFieldLabel names one field for a diff row. The company's form and
// the administrator's review queue both go through here.
func organizationFieldLabel(lang, key string) string {
	return i18n.Translate(lang, "org.profile.field."+key)
}

// orgFieldOrDash keeps an empty side of a diff visible: "" beside a new value
// reads as a missing row rather than as "this was blank".
func orgFieldOrDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}
