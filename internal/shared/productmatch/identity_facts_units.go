package productmatch

import "strings"

func extractUnit(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(t, "ميكروجرام") || strings.Contains(t, "مكجم") || strings.Contains(t, "mcg"):
		return "mcg"
	case strings.Contains(t, "مليجرام") || strings.Contains(t, "ملجرام") || strings.Contains(t, "ملجم") || strings.Contains(t, "مجم") || strings.Contains(t, "محم") || strings.Contains(t, "مج") || strings.Contains(t, "مغ") || strings.Contains(t, "mg"):
		return "mg"
	case strings.Contains(t, "جرام") || strings.Contains(t, "جم") || strings.Contains(t, "gm") || strings.Contains(t, "g"):
		return "g"
	case strings.Contains(t, "وحدة") || strings.Contains(t, "وحده") || strings.Contains(t, "iu"):
		return "iu"
	case strings.Contains(t, "%"):
		return "%"
	case strings.Contains(t, "مللي") || strings.Contains(t, "ملي") || strings.Contains(t, "مل") || strings.Contains(t, "ml"):
		return "ml"
	case strings.Contains(t, "لتر") || strings.Contains(t, "l"):
		return "l"
	}
	return ""
}

func normalizeUnit(u string) string { return extractUnit(u) }
