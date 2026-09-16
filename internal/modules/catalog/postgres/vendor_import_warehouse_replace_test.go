package postgres_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestRetireVariantsExcept_SelectedWarehouseScopingAndFullRemoval(t *testing.T) {
	db := getTestDB(t)
	repo := postgres.NewRepository(db)
	ctx := context.Background()
	sysCtx := database.AsSystem(ctx)

	// 1. Create a vendor organization
	var orgID int64
	err := db.Pool().QueryRow(sysCtx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"مورد اختبار استبدال الكتالوج"}', 'مورد اختبار استبدال الكتالوج', '{"ar":"مورد اختبار استبدال الكتالوج"}', 'TAX-REPLACE-99', 'CR-REPLACE-99', 'vendor', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to create vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(sysCtx, "DELETE FROM org.organizations WHERE id = $1", orgID)
	}()

	// 2. Create two warehouses for this vendor
	var whA, whB int64
	err = db.Pool().QueryRow(sysCtx, `
		INSERT INTO inventory.warehouses (organization_id, name, is_active)
		VALUES ($1, 'مخزن أ - المختار', true)
		RETURNING id
	`, orgID).Scan(&whA)
	if err != nil {
		t.Fatalf("failed to create warehouse A: %v", err)
	}

	err = db.Pool().QueryRow(sysCtx, `
		INSERT INTO inventory.warehouses (organization_id, name, is_active)
		VALUES ($1, 'مخزن ب - غير المختار', true)
		RETURNING id
	`, orgID).Scan(&whB)
	if err != nil {
		t.Fatalf("failed to create warehouse B: %v", err)
	}

	// 3. Get or create a master product
	var prodID int64
	err = db.Pool().QueryRow(sysCtx, `SELECT id FROM catalog.products WHERE deleted_at IS NULL LIMIT 1`).Scan(&prodID)
	if err != nil {
		t.Fatalf("failed to find master product: %v", err)
	}

	// 4. Create 3 variants:
	// Variant 1: in whA only
	// Variant 2: in whA AND whB
	// Variant 3: in whB only
	var v1, v2, v3 int64
	err = db.Pool().QueryRow(sysCtx, `
		INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status)
		VALUES ($1, $2, '{"ar":"صنف مخزن أ فقط"}', 'SKU-WHA-ONLY', 50.00, 'active')
		RETURNING id
	`, orgID, prodID).Scan(&v1)
	if err != nil {
		t.Fatalf("failed to create variant 1: %v", err)
	}

	err = db.Pool().QueryRow(sysCtx, `
		INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status)
		VALUES ($1, $2, '{"ar":"صنف مشترك بالمخزنين"}', 'SKU-SHARED', 60.00, 'active')
		RETURNING id
	`, orgID, prodID).Scan(&v2)
	if err != nil {
		t.Fatalf("failed to create variant 2: %v", err)
	}

	err = db.Pool().QueryRow(sysCtx, `
		INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status)
		VALUES ($1, $2, '{"ar":"صنف مخزن ب فقط"}', 'SKU-WHB-ONLY', 70.00, 'active')
		RETURNING id
	`, orgID, prodID).Scan(&v3)
	if err != nil {
		t.Fatalf("failed to create variant 3: %v", err)
	}

	// 5. Populate stocks:
	// v1 in whA (qty=10)
	// v2 in whA (qty=20) and whB (qty=5)
	// v3 in whB (qty=15)
	_, err = db.Pool().Exec(sysCtx, `
		INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity)
		VALUES
			($1, $2, $4, $5, 10),
			($1, $2, $4, $6, 20),
			($1, $3, $4, $6, 5),
			($1, $3, $4, $7, 15)
	`, orgID, whA, whB, prodID, v1, v2, v3)
	if err != nil {
		t.Fatalf("failed to insert stocks: %v", err)
	}

	// 6. Run RetireVariantsExcept strictly for whA, with keep empty (none kept)
	retired, err := repo.RetireVariantsExcept(sysCtx, orgID, whA, []int64{})
	if err != nil {
		t.Fatalf("RetireVariantsExcept failed: %v", err)
	}

	// Expected: v1 and v2 were in whA, so they were retired for whA.
	// v3 was NOT in whA, so v3 must NOT be in retired!
	retMap := map[int64]bool{}
	for _, r := range retired {
		retMap[r.ID] = true
	}
	if !retMap[v1] {
		t.Errorf("expected v1 to be retired from whA")
	}
	if !retMap[v2] {
		t.Errorf("expected v2 to be retired from whA")
	}
	if retMap[v3] {
		t.Errorf("v3 is in whB only and should NOT have been touched by whA retirement")
	}

	// 7. Verify inventory.stocks:
	// In whA: both v1 and v2 stocks must be GONE (deleted or deleted_at IS NOT NULL)
	var countWhA int
	err = db.Pool().QueryRow(sysCtx, `
		SELECT count(*) FROM inventory.stocks
		WHERE warehouse_id = $1 AND deleted_at IS NULL
	`, whA).Scan(&countWhA)
	if err != nil {
		t.Fatalf("failed to query whA stocks: %v", err)
	}
	if countWhA != 0 {
		t.Errorf("expected 0 active stocks in whA after replacement, got %d", countWhA)
	}

	// In whB: v2 (shared) and v3 must STILL be active and untouched!
	var v2QtyWhB, v3QtyWhB int
	err = db.Pool().QueryRow(sysCtx, `
		SELECT quantity FROM inventory.stocks
		WHERE warehouse_id = $1 AND product_variant_id = $2 AND deleted_at IS NULL
	`, whB, v2).Scan(&v2QtyWhB)
	if err != nil {
		t.Fatalf("expected v2 stock in whB to remain active, got error: %v", err)
	}
	if v2QtyWhB != 5 {
		t.Errorf("expected v2 stock in whB to be 5, got %d", v2QtyWhB)
	}

	err = db.Pool().QueryRow(sysCtx, `
		SELECT quantity FROM inventory.stocks
		WHERE warehouse_id = $1 AND product_variant_id = $2 AND deleted_at IS NULL
	`, whB, v3).Scan(&v3QtyWhB)
	if err != nil {
		t.Fatalf("expected v3 stock in whB to remain active, got error: %v", err)
	}
	if v3QtyWhB != 15 {
		t.Errorf("expected v3 stock in whB to be 15, got %d", v3QtyWhB)
	}

	// 8. Verify catalog.product_variants:
	// Replacing catalog for a warehouse applies strictly to that warehouse's stocks
	// and MUST NOT delete or soft-delete the vendor's product variants from catalog.product_variants.
	// Therefore, v1, v2, and v3 must ALL remain active and preserved in catalog.product_variants.

	// v1 was stocked in whA (now removed from whA), but MUST remain active in catalog.product_variants:
	var v1Deleted bool
	err = db.Pool().QueryRow(sysCtx, `SELECT deleted_at IS NOT NULL FROM catalog.product_variants WHERE id = $1`, v1).Scan(&v1Deleted)
	if err != nil {
		t.Fatalf("failed to query variant v1: %v", err)
	}
	if v1Deleted {
		t.Errorf("variant v1 was soft-deleted from catalog.product_variants, but warehouse replace must preserve catalog variants")
	}

	// v2 was in whA AND whB (still has stock in whB), so it MUST NOT be deleted:
	var v2Deleted bool
	err = db.Pool().QueryRow(sysCtx, `SELECT deleted_at IS NOT NULL FROM catalog.product_variants WHERE id = $1`, v2).Scan(&v2Deleted)
	if err != nil {
		t.Fatalf("failed to query variant v2: %v", err)
	}
	if v2Deleted {
		t.Errorf("variant v2 has active stock in whB and should NOT be deleted from catalog.product_variants")
	}

	// v3 was only in whB, so it MUST NOT be deleted:
	var v3Deleted bool
	err = db.Pool().QueryRow(sysCtx, `SELECT deleted_at IS NOT NULL FROM catalog.product_variants WHERE id = $1`, v3).Scan(&v3Deleted)
	if err != nil {
		t.Fatalf("failed to query variant v3: %v", err)
	}
	if v3Deleted {
		t.Errorf("variant v3 was not in whA and should NOT be deleted from catalog.product_variants")
	}
}
