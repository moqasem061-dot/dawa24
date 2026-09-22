package pages

import (
	"sort"
)

// AnalyticsStatItem represents a sorted metric item with its count and percentage.
type AnalyticsStatItem struct {
	Key     string
	Count   int
	Percent int
}

// SortedStatItems converts a map of frequency counts into a deterministic, descending-sorted slice.
func SortedStatItems(m map[string]int, total int) []AnalyticsStatItem {
	if len(m) == 0 {
		return nil
	}
	items := make([]AnalyticsStatItem, 0, len(m))
	for k, v := range m {
		name := k
		if name == "" {
			name = "غير محدد"
		}
		pct := 0
		if total > 0 {
			pct = (v * 100) / total
			if pct > 100 {
				pct = 100
			}
			if pct < 1 && v > 0 {
				pct = 1
			}
		}
		items = append(items, AnalyticsStatItem{
			Key:     name,
			Count:   v,
			Percent: pct,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	return items
}
