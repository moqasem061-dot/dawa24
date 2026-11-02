package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

// A database that has not had migration 246 applied says so with SQLSTATE 42703
// and the column's name. That is the signal the board falls back on, so it has
// to be recognised precisely: a miss is a 503 on the board for as long as the
// migration is pending, and a false positive silently costs the fast ordering on
// every page view.
func TestMissingUploadDateIsRecognised(t *testing.T) {
	missing := fmt.Errorf("compare postgres: list market discounts: %w",
		&pgconn.PgError{Code: "42703", Message: `column r.file_uploaded_at does not exist`})
	if !isMissingUploadDate(missing) {
		t.Error("the undefined-column error for file_uploaded_at was not recognised; the board would answer 503")
	}

	for name, err := range map[string]error{
		"another missing column": &pgconn.PgError{Code: "42703", Message: `column r.price_after_discount does not exist`},
		"another SQLSTATE":       &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"},
		"a plain error":          errors.New("connection reset"),
	} {
		if isMissingUploadDate(err) {
			t.Errorf("%s was mistaken for a pending migration; the board would fall back for the wrong reason", name)
		}
	}
}

// The fallback ordering is the pre-246 one: the same order, read across the join.
// It has to stay what the old code emitted, because that is the ordering the page
// showed before the denormalised column existed.
func TestMarketLegacyOrderingUsesTheWarehouseDate(t *testing.T) {
	sql, _, _, _ := buildMarketDiscountsOrderedQuery(compare.MarketDiscountsFilter{}, orderByWarehouseDate)
	if !strings.Contains(sql, "ORDER BY f.created_at DESC, r.id DESC") {
		t.Errorf("legacy newest ordering changed:\n%s", sql)
	}
	if strings.Contains(sql, "file_uploaded_at") {
		t.Error("the legacy ordering still names a column the database does not have")
	}

	oldest, _, _, _ := buildMarketDiscountsOrderedQuery(compare.MarketDiscountsFilter{SortBy: "oldest"}, orderByWarehouseDate)
	if !strings.Contains(oldest, "ORDER BY f.created_at ASC, r.id ASC") {
		t.Errorf("legacy oldest ordering changed:\n%s", oldest)
	}
}

// The fallback is a deadline, not a latch.
//
// The deploy's migrate step can run seconds after the image starts, so a process
// that remembered "no column" forever would keep sorting every page view until
// the next restart. After the window the board tries the fast path again — and
// if the migration still has not run, the next request sets the window again.
func TestMarketOrderingHealsAfterTheWindow(t *testing.T) {
	repo := &Repository{}

	repo.legacyUntil.Store(time.Now().Add(legacyOrderingWindow).UnixNano())
	if got := repo.marketOrdering(); got != orderByWarehouseDate {
		t.Error("inside the window the board should stay on the fallback ordering")
	}

	repo.legacyUntil.Store(time.Now().Add(-time.Second).UnixNano())
	if got := repo.marketOrdering(); got != orderByUploadDate {
		t.Error("once the window has passed the board should try the denormalised column again")
	}

	if got := (&Repository{}).marketOrdering(); got != orderByUploadDate {
		t.Error("a database that never reported a missing column must use the index-backed ordering")
	}
}

// Every other sort is schema-independent: it reads columns that have always
// existed, so the fallback must not change it.
func TestMarketOtherSortsIgnoreTheOrderingChoice(t *testing.T) {
	for _, sortBy := range []string{"price_asc", "price_desc", "discount_asc", "discount_desc"} {
		current, _, _, _ := buildMarketDiscountsOrderedQuery(compare.MarketDiscountsFilter{SortBy: sortBy}, orderByUploadDate)
		legacy, _, _, _ := buildMarketDiscountsOrderedQuery(compare.MarketDiscountsFilter{SortBy: sortBy}, orderByWarehouseDate)
		if current != legacy {
			t.Errorf("sort %q differs by schema, but it reads no new column:\ncurrent:\n%s\nlegacy:\n%s",
				sortBy, current, legacy)
		}
	}
}
