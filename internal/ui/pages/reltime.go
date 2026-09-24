package pages

import (
	"fmt"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// RelTime renders an Arabic relative time for notification timestamps.
func RelTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("ar", "time.rel.now")
	case d < time.Hour:
		return fmt.Sprintf(i18n.T("ar", "time.rel.minutes_ago"), int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf(i18n.T("ar", "time.rel.hours_ago"), int(d.Hours()))
	default:
		return fmt.Sprintf(i18n.T("ar", "time.rel.days_ago"), int(d.Hours()/24))
	}
}
