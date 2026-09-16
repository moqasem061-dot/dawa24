package datasets

import (
	"encoding/json"
)

// Result is what a plan returned, with values already in the shape the model
// and an export file see.
type Result struct {
	Columns []Column
	// Rows holds one value per column: json.Number for numbers (exact, never a
	// float), bool, string, or nil.
	Rows [][]any
	// Keys holds each row's record id when the plan HasKey. Ids never leave the
	// server as themselves; tools turn them into handles and links.
	Keys []int64
	// Total is how many rows (or groups) the request matches before paging.
	Total int
}

// Value converts one scanned text value by its column type.
func Value(t Type, s *string) any {
	if s == nil {
		return nil
	}
	switch t {
	case Int, Number, Money, Ref:
		return json.Number(*s)
	case Bool:
		return *s == "true"
	default:
		return *s
	}
}
