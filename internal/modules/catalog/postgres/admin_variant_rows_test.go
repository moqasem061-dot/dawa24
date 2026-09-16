package postgres_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
)

// The cross-supplier stock listing has to be answerable, and its pager has to
// be honest: the count and the page filter over the same relations, so a filter
// that narrows one narrows the other.
func TestListAdminVariantRows_FiltersNarrowTheCount(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	repo := postgres.NewRepository(db)

	all, total, err := repo.ListAdminVariantRows(ctx, catalog.AdminVariantFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListAdminVariantRows: %v", err)
	}
	if total == 0 {
		t.Skip("no variants on this database")
	}
	if len(all) > 5 {
		t.Fatalf("limit ignored: got %d rows for limit 5", len(all))
	}

	// A supplier that does not exist must count zero rather than being ignored.
	// A positive id, because a non-positive one is deliberately read as "no
	// filter" -- the handler only ever passes ids it parsed as positive.
	_, none, err := repo.ListAdminVariantRows(ctx, catalog.AdminVariantFilter{
		OrganizationID: 999999999, Limit: 5,
	})
	if err != nil {
		t.Fatalf("ListAdminVariantRows with unknown supplier: %v", err)
	}
	if none != 0 {
		t.Errorf("an unknown supplier must count zero, got %d", none)
	}

	// The in-stock and out-of-stock partitions must sum to the whole, or one of
	// them is silently dropping rows.
	_, inStock, err := repo.ListAdminVariantRows(ctx, catalog.AdminVariantFilter{Stock: "in", Limit: 1})
	if err != nil {
		t.Fatalf("in-stock filter: %v", err)
	}
	_, outStock, err := repo.ListAdminVariantRows(ctx, catalog.AdminVariantFilter{Stock: "out", Limit: 1})
	if err != nil {
		t.Fatalf("out-of-stock filter: %v", err)
	}
	if inStock+outStock != total {
		t.Errorf("in (%d) + out (%d) = %d, want the unfiltered total %d",
			inStock, outStock, inStock+outStock, total)
	}

	// The warehouse breakdown must reconcile with the total the same row
	// reports, or the screen shows two different numbers for one holding.
	for _, row := range all {
		if len(row.Warehouses) == 0 {
			continue
		}
		sum := 0
		for _, wh := range row.Warehouses {
			sum += wh.Quantity
		}
		if sum != row.TotalQuantity {
			t.Errorf("variant %d: warehouses sum to %d but the row reports %d",
				row.VariantID, sum, row.TotalQuantity)
		}
	}
}
