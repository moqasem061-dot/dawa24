package ui

import (
	"strconv"
	"strings"
)

// csvSafe stops a spreadsheet from running a text cell as a formula. A cell
// that starts with = + - @ tab or CR gets a leading apostrophe, so
// "=HYPERLINK(...)" opens as text. A plain number such as -12.50 (a negative
// price) is not a formula and is returned unchanged.
func csvSafe(s string) string {
	if s == "" || !strings.ContainsAny(s[:1], "=+-@\t\r") {
		return s
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s
	}
	return "'" + s
}
