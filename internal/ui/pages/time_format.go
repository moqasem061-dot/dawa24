package pages

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

// Standard 12-hour datetime layouts with PM/AM.
const (
	LayoutDateTime12    = "2006-01-02 03:04 PM"
	LayoutDateTimeSec12 = "2006-01-02 03:04:05 PM"
	LayoutTime12        = "03:04 PM"
	LayoutTimeSec12     = "03:04:05 PM"
	LayoutDateOnly      = "2006-01-02"
)

// FormatDateTime formats a timestamp in 12-hour format: YYYY-MM-DD hh:mm PM (in Africa/Cairo timezone).
func FormatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timeutil.InCairo(t).Format(LayoutDateTime12)
}

// FormatDateTimePtr formats a pointer timestamp in 12-hour format: YYYY-MM-DD hh:mm PM (in Africa/Cairo timezone).
func FormatDateTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return timeutil.InCairo(*t).Format(LayoutDateTime12)
}

// FormatDateTimeSec formats a timestamp in 12-hour format with seconds: YYYY-MM-DD hh:mm:ss PM (in Africa/Cairo timezone).
func FormatDateTimeSec(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timeutil.InCairo(t).Format(LayoutDateTimeSec12)
}

// FormatTime12 formats time-only in 12-hour format: hh:mm PM (in Africa/Cairo timezone).
func FormatTime12(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timeutil.InCairo(t).Format(LayoutTime12)
}

// FormatTimeSec12 formats time-only with seconds in 12-hour format: hh:mm:ss PM (in Africa/Cairo timezone).
func FormatTimeSec12(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timeutil.InCairo(t).Format(LayoutTimeSec12)
}

// FormatDateOnly formats a date in YYYY-MM-DD format (in Africa/Cairo timezone).
func FormatDateOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return timeutil.InCairo(t).Format(LayoutDateOnly)
}

// FormatClock12 converts a stored 24-hour clock string ("HH:MM" or "HH:MM:SS")
// to the Egyptian 12-hour form the UI shows everywhere times are read back:
// "16:00" -> "4:00 م", "09:30" -> "9:30 ص", "00:15" -> "12:15 ص". A value that
// is empty or cannot be parsed is returned unchanged.
func FormatClock12(clock string) string {
	s := strings.TrimSpace(clock)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return clock
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return clock
	}
	suffix := "ص"
	if h >= 12 {
		suffix = "م"
	}
	h12 := h % 12
	if h12 == 0 {
		h12 = 12
	}
	return fmt.Sprintf("%d:%02d %s", h12, m, suffix)
}

// FormatCoverageWindow renders a weekly-coverage delivery window in the Egyptian
// 12-hour form, or the all-day label when no window was set (both ends empty).
// from/to are the nullable "HH:MM" strings the workflow repository returns.
func FormatCoverageWindow(from, to *string) string {
	f, t := "", ""
	if from != nil {
		f = strings.TrimSpace(*from)
	}
	if to != nil {
		t = strings.TrimSpace(*to)
	}
	switch {
	case f == "" && t == "":
		return "طوال اليوم (24 ساعة)"
	case f != "" && t != "":
		return FormatClock12(f) + " – " + FormatClock12(t)
	case f != "":
		return "من " + FormatClock12(f)
	default:
		return "حتى " + FormatClock12(t)
	}
}
