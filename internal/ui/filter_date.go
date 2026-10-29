package ui

import "time"

func validFilterDate(raw string) string {
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return ""
	}
	return raw
}
