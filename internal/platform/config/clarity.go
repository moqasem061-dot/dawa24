package config

import (
	"regexp"
	"strings"
)

// Clarity configures Microsoft Clarity session analytics (heatmaps and
// recordings) on every page rendered by the layouts.
//
// CLARITY_PROJECT_ID is the id from the Clarity dashboard (Settings → Setup).
// Empty disables it entirely: no script, and the Content-Security-Policy is not
// widened. That is the default, so development and test runs never record into
// the production project; set it on the deployed environment.
type Clarity struct {
	ProjectID string
}

// Enabled reports whether the tag should be rendered.
func (c Clarity) Enabled() bool { return c.ProjectID != "" }

// clarityProjectID is the shape of a Clarity project id. The id is written
// into a script URL, so it is checked rather than trusted.
var clarityProjectID = regexp.MustCompile(`^[a-z0-9]{6,32}$`)

func loadClarity(fail func(string, ...any)) Clarity {
	id := strings.ToLower(strings.TrimSpace(getStr("CLARITY_PROJECT_ID", "")))
	if id != "" && !clarityProjectID.MatchString(id) {
		fail("CLARITY_PROJECT_ID must be 6-32 lowercase letters or digits, got %q", id)
		return Clarity{}
	}
	return Clarity{ProjectID: id}
}
