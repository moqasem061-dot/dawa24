package pages

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// Helpers for the column setup wizard (compare_mapping.templ).

// detectionCol picks one field's detected column index, or nil when the
// detector had no opinion.
func detectionCol(d *compare.ColumnDetection, field string) *int {
	if d == nil {
		return nil
	}
	switch field {
	case "name":
		return d.NameCol
	case "price":
		return d.PriceCol
	case "discount":
		return d.DiscountCol
	case "code":
		return d.CodeCol
	}
	return nil
}

// detectionScore is how sure the detector was about one field, 0 when unknown.
func detectionScore(d *compare.ColumnDetection, field compare.TargetField) float64 {
	if d == nil || d.FieldScores == nil {
		return 0
	}
	return d.FieldScores[field]
}

// cmapSelected decides which option is pre-chosen: what the user saved last
// time if there is one, otherwise what the detector guessed.
//
// A saved mapping always wins. A user who corrected the detector once must not
// find their correction undone every time they reopen the file.
func cmapSelected(saved, detected *int, idx int) bool {
	if saved != nil {
		return *saved == idx
	}
	return detected != nil && *detected == idx
}

// cmapScoreTone grades the detector's confidence, so a weak guess does not look
// as trustworthy as a strong one.
func cmapScoreTone(score float64) string {
	switch {
	case score >= 0.85:
		return "is-strong"
	case score >= 0.6:
		return "is-likely"
	}
	return "is-weak"
}

// cmapHeaderLabel names a column, falling back to its position when the file
// gave no header text.
func cmapHeaderLabel(h string) string {
	if s := strings.TrimSpace(h); s != "" {
		return s
	}
	return "(بدون عنوان)"
}

// padPreviewCells pads a short row out to the header count so trailing blanks
// do not shift the remaining values under the wrong columns.
func padPreviewCells(row []string, width int) []string {
	if len(row) >= width {
		return row[:width]
	}
	out := make([]string, width)
	copy(out, row)
	return out
}

// progressWidthClass maps a step to one of the fixed .progress-pct-* classes,
// because a width belongs in a stylesheet and `make check-inline-styles`
// enforces that.
func progressWidthClass(step, total int) string {
	if total <= 0 {
		return "progress-pct-0"
	}
	pct := step * 100 / total
	buckets := []int{0, 10, 20, 33, 50, 66, 75, 80, 100}
	best := 0
	for _, b := range buckets {
		if b <= pct {
			best = b
		}
	}
	return fmt.Sprintf("progress-pct-%d", best)
}
