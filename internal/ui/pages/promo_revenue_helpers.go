package pages

import "regexp"

var promoHexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{3}([0-9A-Fa-f]{3})?$`)

// promoBadgeColor is a package's badge colour for data-bg / data-tint, with
// the default when it is unset or not a plain hex colour.
func promoBadgeColor(c string) string {
	if promoHexColor.MatchString(c) {
		return c
	}
	return "#0284c7"
}
