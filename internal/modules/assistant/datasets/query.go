package datasets

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Request is a question about one dataset, exactly as the model may ask it.
// Every string in it is a name to look up or a value to bind, never SQL.
type Request struct {
	Dataset string   `json:"dataset"`
	Fields  []string `json:"fields,omitempty"`
	Filters []Filter `json:"filters,omitempty"`
	Search  string   `json:"search,omitempty"`
	// GroupBy entries are field names, with a time grain for dates:
	// "status", "created_at:month".
	GroupBy []string `json:"group_by,omitempty"`
	// Metrics are "count", "count_distinct:field", "sum:field", "avg:field",
	// "min:field", "max:field".
	Metrics []string `json:"metrics,omitempty"`
	Sort    []Sort   `json:"sort,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	Offset  int      `json:"offset,omitempty"`
}

// Filter is one condition.
type Filter struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Sort orders the result by a field (row mode), or by a group or metric name
// (aggregate mode).
type Sort struct {
	By  string `json:"by"`
	Dir string `json:"dir,omitempty"`
}

// Aggregate reports whether the request asks for grouped or summarised
// results rather than rows.
func (r Request) Aggregate() bool { return len(r.GroupBy) > 0 || len(r.Metrics) > 0 }

// Bounds on what one request may ask for. They keep a single question from
// becoming an expensive statement; none of them limits how many rows an
// aggregate reads.
const (
	MaxFields      = 30
	MaxFilters     = 15
	MaxInValues    = 100
	MaxGroupBy     = 3
	MaxMetrics     = 8
	MaxSorts       = 3
	MaxSearchChars = 120
	MaxValueChars  = 200
)

var (
	// ErrInvalid is a request the model can correct: the message says what.
	ErrInvalid = errors.New("invalid dataset request")
	// ErrHandle is a reference that did not verify. It says nothing more.
	ErrHandle = errors.New("invalid reference")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}
