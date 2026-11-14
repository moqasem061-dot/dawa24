package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The product filter has to be able to use the catalogue-link index of
// migration 243 (compare_file_rows_matched_product). A small fixture makes the
// planner prefer a scan whatever the query looks like, so sequential scans are
// switched off for the statement: what is asserted is that the filter's shape
// lets the planner reach that index at all, for the listing and for the count.
func TestMarketProductFilterCanUseTheCatalogueLinkIndex(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.Open(ctx, config.Database{URL: dbURL, MaxConns: 2, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 10 * time.Second})
	if err != nil {
		t.Skipf("cannot connect to database: %v", err)
	}
	t.Cleanup(db.Close)

	product := int64(1)
	filter := compare.MarketDiscountsFilter{ProductID: &product, Limit: 24}
	listing, listingArgs, _, _ := buildMarketDiscountsQuery(filter)
	count, countArgs := buildMarketDiscountsCountQuery(filter)
	for name, q := range map[string]struct {
		sql  string
		args []any
	}{"listing": {listing, listingArgs}, "count": {count, countArgs}} {
		var plan strings.Builder
		err := db.InReadTx(database.AsSystem(ctx), func(c context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(c, `SET LOCAL enable_seqscan = off`); err != nil {
				return err
			}
			rows, err := tx.Query(c, "EXPLAIN "+q.sql, q.args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					return err
				}
				plan.WriteString(line + "\n")
			}
			return rows.Err()
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(plan.String(), "compare_file_rows_matched_product") {
			t.Errorf("%s: the product filter does not reach compare_file_rows_matched_product:\n%s", name, plan.String())
		}
	}
}
