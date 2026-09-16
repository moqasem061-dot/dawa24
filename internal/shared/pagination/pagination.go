// Package pagination provides standard keyset (cursor-based) and offset pagination
// for all API list endpoints across the platform.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// DefaultLimit is the fallback page size if not specified.
const DefaultLimit = 50

// MaxLimit is the maximum permissible page size ceiling.
const MaxLimit = 200

// TableRows is how many rows a dashboard table shows per page.
//
// The list screens had picked their own: brands and reference data showed 25,
// chat history and users showed 50. Same table chrome, same filter bar, twice
// the scroll depending on which link you followed -- and a reader who has
// learned "the second page starts at 26" on one screen is wrong on the next.
//
// This is deliberately not DefaultLimit. DefaultLimit (50) is the API's answer
// for a machine reading a collection; this is the answer for a person reading
// rows on a screen, and the two do not have to agree.
const TableRows = 25

// RowsPerPage reads the reader's chosen page size off the request.
//
// The rows-per-page control on every dashboard table submits ?limit=N. Before
// this existed each handler parsed it its own way: one defaulted to 50 and
// capped at 500, the next defaulted to 25 and capped at 100, and a third
// ignored the parameter altogether, so the same control did three different
// things depending on which table it sat under.
//
// A value outside the offered set falls back to TableRows rather than being
// clamped to the nearest bound: the query string is user-supplied, and honouring
// ?limit=97 would let a caller ask for page sizes the UI never offers.
func RowsPerPage(r *http.Request) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return TableRows
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return TableRows
	}
	for _, allowed := range RowsPerPageOptions {
		if n == allowed {
			return n
		}
	}
	return TableRows
}

// RowsPerPageOptions is the set the control offers, and the only set
// RowsPerPage will honour.
var RowsPerPageOptions = []int{10, 25, 50, 100}

// PageNumber reads ?page=, clamped to at least 1.
func PageNumber(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// Params captures parsed client pagination request parameters.
type Params struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// Normalize ensures bounds on limit and sets defaults.
func (p *Params) Normalize() {
	if p.Limit <= 0 {
		p.Limit = DefaultLimit
	} else if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}

// FromRequest extracts pagination parameters from an HTTP query string.
func FromRequest(r *http.Request) Params {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	cursor := q.Get("cursor")

	p := Params{
		Limit:  limit,
		Cursor: cursor,
		Offset: offset,
	}
	p.Normalize()
	return p
}

// Page represents a paginated API response payload.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	TotalCount *int64 `json:"total_count,omitempty"`
}

// CursorPayload represents internal data packed into an opaque cursor token.
type CursorPayload struct {
	ID        int64  `json:"id"`
	SortValue string `json:"sort_val,omitempty"`
}

// EncodeCursor creates an opaque base64 cursor token.
func EncodeCursor(id int64, sortVal string) string {
	payload := CursorPayload{ID: id, SortValue: sortVal}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque base64 cursor token.
func DecodeCursor(token string) (int64, string, error) {
	if token == "" {
		return 0, "", nil
	}
	b, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var payload CursorPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return 0, "", fmt.Errorf("invalid cursor payload: %w", err)
	}
	return payload.ID, payload.SortValue, nil
}
