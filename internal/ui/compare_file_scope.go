package ui

import "github.com/moqasem061-dot/dawa24/internal/modules/compare"

func containsCompareFile(files []*compare.CompareFile, id int64) bool {
	for _, file := range files {
		if file != nil && file.ID == id {
			return true
		}
	}
	return false
}
