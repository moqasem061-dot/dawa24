package postgres_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
)

// The cross-tenant warehouse listing has to page honestly and its holdings have
// to reconcile: the screen decides whether disabling a warehouse strands goods
// from the numbers this query returns.
func TestListAdminWarehouseRows(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	repo := postgres.NewRepository(db)

	rows, total, err := repo.ListAdminWarehouseRows(ctx, inventory.AdminWarehouseFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListAdminWarehouseRows: %v", err)
	}
	if total == 0 {
		t.Skip("no warehouses on this database")
	}
	if len(rows) > 5 {
		t.Fatalf("limit ignored: got %d rows for limit 5", len(rows))
	}

	// The active and inactive partitions must sum to the whole, or the status
	// filter is silently dropping rows.
	yes, no := true, false
	_, activeTotal, err := repo.ListAdminWarehouseRows(ctx, inventory.AdminWarehouseFilter{Active: &yes, Limit: 1})
	if err != nil {
		t.Fatalf("active filter: %v", err)
	}
	_, inactiveTotal, err := repo.ListAdminWarehouseRows(ctx, inventory.AdminWarehouseFilter{Active: &no, Limit: 1})
	if err != nil {
		t.Fatalf("inactive filter: %v", err)
	}
	if activeTotal+inactiveTotal != total {
		t.Errorf("active (%d) + inactive (%d) = %d, want the unfiltered total %d",
			activeTotal, inactiveTotal, activeTotal+inactiveTotal, total)
	}

	// A warehouse holding nothing must not report a total quantity, and one
	// holding something must report items alongside it. The screen warns about
	// stranding stock from exactly these two numbers.
	for _, row := range rows {
		if row.TotalQuantity != 0 && row.ItemCount == 0 {
			t.Errorf("warehouse %d reports quantity %d across zero items",
				row.ID, row.TotalQuantity)
		}
		if row.HoldsStock() != (row.TotalQuantity > 0) {
			t.Errorf("warehouse %d: HoldsStock disagrees with TotalQuantity %d",
				row.ID, row.TotalQuantity)
		}
	}

	owners, err := repo.AdminWarehouseOwners(ctx)
	if err != nil {
		t.Fatalf("AdminWarehouseOwners: %v", err)
	}
	if len(owners) == 0 {
		t.Error("warehouses exist but no organisation owns one")
	}
	for _, o := range owners {
		if o.Count <= 0 {
			t.Errorf("owner %d listed with %d warehouses", o.ID, o.Count)
		}
	}
}
