// Package format provides unified number, currency, distance, and date
// formatting across the entire Dawa24 platform.
package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

// Money formats a monetary amount with thousands separators and proper currency suffix.
// Example: 1234567.89 -> "1,234,567.89 ج.م" (ar) or "1,234,567.89 EGP" (en).
func Money(a money.Amount, lang string) string {
	minor := a.Minor()
	isNegative := minor < 0
	if isNegative {
		minor = -minor
	}

	whole := minor / 100
	cents := minor % 100

	wholeStr := Integer(whole, lang)
	formatted := fmt.Sprintf("%s.%02d", wholeStr, cents)
	if isNegative {
		formatted = "-" + formatted
	}

	if lang == "en" {
		return formatted + " EGP"
	}
	return formatted + " ج.م"
}

// Integer formats an integer with comma grouping (e.g. 1234567 -> "1,234,567").
func Integer(n int64, lang string) string {
	isNeg := n < 0
	if isNeg {
		n = -n
	}
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		if isNeg {
			return "-" + str
		}
		return str
	}

	var sb strings.Builder
	offset := len(str) % 3
	if offset > 0 {
		sb.WriteString(str[:offset])
		if len(str) > offset {
			sb.WriteByte(',')
		}
	}
	for i := offset; i < len(str); i += 3 {
		if i > offset {
			sb.WriteByte(',')
		}
		sb.WriteString(str[i : i+3])
	}

	res := sb.String()
	if isNeg {
		res = "-" + res
	}
	return res
}

// Distance formats integer metres as metres under 1 km, or kilometres with one decimal place.
// Example: 850 -> "850 م", 3200 -> "3.2 كم"
func Distance(metres int, lang string) string {
	if metres < 1000 {
		if lang == "en" {
			return fmt.Sprintf("%d m", metres)
		}
		return fmt.Sprintf("%d م", metres)
	}

	km := float64(metres) / 1000.0
	if lang == "en" {
		return fmt.Sprintf("%.1f km", km)
	}
	return fmt.Sprintf("%.1f كم", km)
}

// Arabic month names for standard Egyptian/Gregorian calendar.
var arMonths = [...]string{
	"", "يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو",
	"يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر",
}

// Date formats a date (e.g. "17 أغسطس 2026" or "Aug 17, 2026").
func Date(t time.Time, lang string) string {
	if t.IsZero() {
		return ""
	}
	t = timeutil.InCairo(t)
	if lang == "en" {
		return t.Format("Jan 02, 2006")
	}
	m := int(t.Month())
	if m >= 1 && m <= 12 {
		return fmt.Sprintf("%d %s %d", t.Day(), arMonths[m], t.Year())
	}
	return t.Format("2006-01-02")
}

// DateTime formats date and time in 12-hour AM/PM format (e.g. "17 أغسطس 2026 02:30 PM" or "Aug 17, 2026 02:30 PM").
func DateTime(t time.Time, lang string) string {
	if t.IsZero() {
		return ""
	}
	t = timeutil.InCairo(t)
	if lang == "en" {
		return t.Format("Jan 02, 2006 03:04 PM")
	}
	m := int(t.Month())
	period := "AM"
	hour := t.Hour()
	if hour >= 12 {
		period = "PM"
	}
	h12 := hour % 12
	if h12 == 0 {
		h12 = 12
	}
	if m >= 1 && m <= 12 {
		return fmt.Sprintf("%d %s %d %02d:%02d %s", t.Day(), arMonths[m], t.Year(), h12, t.Minute(), period)
	}
	return t.Format("2006-01-02 03:04 PM")
}

// Bytes formats byte size into human readable string (e.g. 500 B, 1.5 KB, 2.4 MB).
func Bytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
