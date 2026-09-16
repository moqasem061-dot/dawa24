package catalog

import (
	"strings"
)

// FirstWordOf returns the query's first word, stripped of surrounding
// punctuation, for first-word-prioritized search ranking.
//
// Arabic product names lead with the brand ("ارت كيو كريم 40 جم"), so when a
// pharmacy links a saving-list row, candidates starting with the query's
// first word are almost always the right family. Single-word queries return
// the word itself; empty queries return "" (legacy ranking).
func FirstWordOf(query string) string {
	for _, field := range strings.Fields(query) {
		if w := strings.Trim(field, ".,،;:()[]{}\"'«»!?؟-–—/\\"); w != "" {
			return w
		}
	}
	return ""
}
