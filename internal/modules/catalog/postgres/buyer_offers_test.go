package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestListBuyerOffers_Integration(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var (
		supplierAOrgID    int64
		supplierBOrgID    int64
		buyerOrgID        int64
		supplierABranchID int64
		supplierBBranchID int64
		buyerBranchID     int64
		productID         int64
		variantA          int64
		variantB          int64
		variantBuyer      int64
		workConnectedID   int64
		workUnconnID      int64
		whA               int64
		whB               int64
		whBuyer           int64
	)

	// Seed test data in system context
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		randSuffix := time.Now().UnixNano() % 1000000

		// 1. Create 2 supplier orgs + 1 buyer org
		err := tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (name, type, status, legal_name, trade_name)
			VALUES ($1, 'vendor', 'approved', $2, $1) RETURNING id
		`, fmt.Sprintf(`{"ar": "مورد أ %d", "en": "Vendor A %d"}`, randSuffix, randSuffix),
			fmt.Sprintf("Vendor A %d", randSuffix)).Scan(&supplierAOrgID)
		if err != nil {
			return fmt.Errorf("seed supplier A: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (name, type, status, legal_name, trade_name)
			VALUES ($1, 'vendor', 'approved', $2, $1) RETURNING id
		`, fmt.Sprintf(`{"ar": "مورد ب %d", "en": "Vendor B %d"}`, randSuffix, randSuffix),
			fmt.Sprintf("Vendor B %d", randSuffix)).Scan(&supplierBOrgID)
		if err != nil {
			return fmt.Errorf("seed supplier B: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (name, type, status, legal_name, trade_name)
			VALUES ($1, 'pharmacy', 'approved', $2, $1) RETURNING id
		`, fmt.Sprintf(`{"ar": "صيدلية المشتري %d", "en": "Buyer %d"}`, randSuffix, randSuffix),
			fmt.Sprintf("Buyer %d", randSuffix)).Scan(&buyerOrgID)
		if err != nil {
			return fmt.Errorf("seed buyer org: %w", err)
		}

		// 2. Create branches
		err = tx.QueryRow(txCtx, `
			INSERT INTO org.branches (organization_id, name, code, status)
			VALUES ($1, '{"ar": "فرع أ", "en": "Branch A"}', $2, 'active') RETURNING id
		`, supplierAOrgID, fmt.Sprintf("BRA-%d", randSuffix)).Scan(&supplierABranchID)
		if err != nil {
			return fmt.Errorf("seed branch A: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO org.branches (organization_id, name, code, status)
			VALUES ($1, '{"ar": "فرع ب", "en": "Branch B"}', $2, 'active') RETURNING id
		`, supplierBOrgID, fmt.Sprintf("BRB-%d", randSuffix)).Scan(&supplierBBranchID)
		if err != nil {
			return fmt.Errorf("seed branch B: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO org.branches (organization_id, name, code, status)
			VALUES ($1, '{"ar": "فرع المشتري", "en": "Buyer Branch"}', $2, 'active') RETURNING id
		`, buyerOrgID, fmt.Sprintf("BRC-%d", randSuffix)).Scan(&buyerBranchID)
		if err != nil {
			return fmt.Errorf("seed buyer branch: %w", err)
		}

		// 3. Create institutional works
		err = tx.QueryRow(txCtx, `
			INSERT INTO org.institutional_works (title, slug)
			VALUES ('{"ar": "عمل متصل", "en": "Work Connected"}', $1) RETURNING id
		`, fmt.Sprintf("work-conn-%d", randSuffix)).Scan(&workConnectedID)
		if err != nil {
			return fmt.Errorf("seed work connected: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO org.institutional_works (title, slug)
			VALUES ('{"ar": "عمل غير متصل", "en": "Work Unconnected"}', $1) RETURNING id
		`, fmt.Sprintf("work-unconn-%d", randSuffix)).Scan(&workUnconnID)
		if err != nil {
			return fmt.Errorf("seed work unconn: %w", err)
		}

		// Assign workConnected to supplier A branch, and workUnconn to supplier B branch
		_, err = tx.Exec(txCtx, `
			INSERT INTO org.branch_institutional_works (branch_id, institutional_work_id, work_category)
			VALUES ($1, $2, 'group'), ($3, $4, 'group')
		`, supplierABranchID, workConnectedID, supplierBBranchID, workUnconnID)
		if err != nil {
			return fmt.Errorf("seed branch works: %w", err)
		}

		// 4. Create master product
		err = tx.QueryRow(txCtx, `
			INSERT INTO catalog.products (organization_id, name, sku, price)
			VALUES ($1, '{"ar": "منتج تجريبي", "en": "Test Product"}', $2, 100.00) RETURNING id
		`, supplierAOrgID, fmt.Sprintf("SKU-P-%d", randSuffix)).Scan(&productID)
		if err != nil {
			return fmt.Errorf("seed master product: %w", err)
		}

		// 5. Create variants: Variant A (Supplier A), Variant B (Supplier B), Variant Buyer (Buyer Org)
		err = tx.QueryRow(txCtx, `
			INSERT INTO catalog.product_variants (product_id, organization_id, branch_id, name, sku, price, status)
			VALUES ($1, $2, $3, '{"ar": "عرض أ", "en": "Offer A"}', $4, 90.00, 'active') RETURNING id
		`, productID, supplierAOrgID, supplierABranchID, fmt.Sprintf("SKU-VA-%d", randSuffix)).Scan(&variantA)
		if err != nil {
			return fmt.Errorf("seed variant A: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO catalog.product_variants (product_id, organization_id, branch_id, name, sku, price, status)
			VALUES ($1, $2, $3, '{"ar": "عرض ب", "en": "Offer B"}', $4, 85.00, 'active') RETURNING id
		`, productID, supplierBOrgID, supplierBBranchID, fmt.Sprintf("SKU-VB-%d", randSuffix)).Scan(&variantB)
		if err != nil {
			return fmt.Errorf("seed variant B: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO catalog.product_variants (product_id, organization_id, branch_id, name, sku, price, status)
			VALUES ($1, $2, $3, '{"ar": "عرض المشتري", "en": "Offer Buyer"}', $4, 80.00, 'active') RETURNING id
		`, productID, buyerOrgID, buyerBranchID, fmt.Sprintf("SKU-VC-%d", randSuffix)).Scan(&variantBuyer)
		if err != nil {
			return fmt.Errorf("seed variant Buyer: %w", err)
		}

		// 6. Create warehouses & add inventory stocks
		err = tx.QueryRow(txCtx, `
			INSERT INTO inventory.warehouses (organization_id, branch_id, name, code)
			VALUES ($1, $2, 'WH A', $3) RETURNING id
		`, supplierAOrgID, supplierABranchID, fmt.Sprintf("WHA-%d", randSuffix)).Scan(&whA)
		if err != nil {
			return fmt.Errorf("seed warehouse A: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO inventory.warehouses (organization_id, branch_id, name, code)
			VALUES ($1, $2, 'WH B', $3) RETURNING id
		`, supplierBOrgID, supplierBBranchID, fmt.Sprintf("WHB-%d", randSuffix)).Scan(&whB)
		if err != nil {
			return fmt.Errorf("seed warehouse B: %w", err)
		}

		err = tx.QueryRow(txCtx, `
			INSERT INTO inventory.warehouses (organization_id, branch_id, name, code)
			VALUES ($1, $2, 'WH Buyer', $3) RETURNING id
		`, buyerOrgID, buyerBranchID, fmt.Sprintf("WHC-%d", randSuffix)).Scan(&whBuyer)
		if err != nil {
			return fmt.Errorf("seed warehouse Buyer: %w", err)
		}

		_, err = tx.Exec(txCtx, `
			INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity)
			VALUES ($1, $2, $10, $3, 50), ($4, $5, $10, $6, 30), ($7, $8, $10, $9, 20)
		`, supplierAOrgID, whA, variantA, supplierBOrgID, whB, variantB, buyerOrgID, whBuyer, variantBuyer, productID)
		if err != nil {
			return fmt.Errorf("seed stocks: %w", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to seed test data: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM inventory.stocks WHERE product_variant_id IN ($1, $2, $3)", variantA, variantB, variantBuyer)
			_, _ = tx.Exec(txCtx, "DELETE FROM inventory.warehouses WHERE id IN ($1, $2, $3)", whA, whB, whBuyer)
			_, _ = tx.Exec(txCtx, "DELETE FROM catalog.product_variants WHERE id IN ($1, $2, $3)", variantA, variantB, variantBuyer)
			_, _ = tx.Exec(txCtx, "DELETE FROM catalog.products WHERE id = $1", productID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.branch_institutional_works WHERE branch_id IN ($1, $2, $3)", supplierABranchID, supplierBBranchID, buyerBranchID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.institutional_works WHERE id IN ($1, $2)", workConnectedID, workUnconnID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.branches WHERE id IN ($1, $2, $3)", supplierABranchID, supplierBBranchID, buyerBranchID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.organizations WHERE id IN ($1, $2, $3)", supplierAOrgID, supplierBOrgID, buyerOrgID)
			return nil
		})
	})

	repo := postgres.NewRepository(db)

	// Test 1: Buyer queries with AllowedWorkIDs containing only workConnectedID.
	// Expected: Only variantA returned.
	// - variantB excluded because supplierB holds workUnconnID.
	// - variantBuyer excluded because of own-org exclusion (BuyerOrgID).
	offers, total, err := repo.ListBuyerOffers(ctx, catalog.BuyerOfferQuery{
		BuyerOrgID:     buyerOrgID,
		BuyerBranchID:  buyerBranchID,
		AllowedWorkIDs: []int64{workConnectedID},
		OnlyInStock:    true,
		Limit:          10,
		Offset:         0,
	})
	if err != nil {
		t.Fatalf("ListBuyerOffers failed: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected total >= 1, got %d", total)
	}

	foundA := false
	for _, o := range offers {
		if o.VariantID == variantA {
			foundA = true
		}
		if o.VariantID == variantB {
			t.Errorf("expected variant B to be filtered out by institutional connection mismatch")
		}
		if o.VariantID == variantBuyer {
			t.Errorf("expected buyer's own variant to be filtered out by own-org exclusion")
		}
	}
	if !foundA {
		t.Errorf("expected variant A to be found in buyer offers")
	}

	// Test 2: Unconnected buyer branch (AllowedWorkIDs is empty)
	// Expected: 0 offers returned.
	emptyOffers, emptyTotal, err := repo.ListBuyerOffers(ctx, catalog.BuyerOfferQuery{
		BuyerOrgID:     buyerOrgID,
		BuyerBranchID:  buyerBranchID,
		AllowedWorkIDs: []int64{},
		OnlyInStock:    true,
		Limit:          10,
		Offset:         0,
	})
	if err != nil {
		t.Fatalf("ListBuyerOffers with empty works failed: %v", err)
	}
	if emptyTotal != 0 || len(emptyOffers) != 0 {
		t.Errorf("expected 0 offers for branch with no connected works, got %d (len %d)", emptyTotal, len(emptyOffers))
	}

	// Test 3: Pagination slicing
	pagedOffers, pagedTotal, err := repo.ListBuyerOffers(ctx, catalog.BuyerOfferQuery{
		BuyerOrgID:     buyerOrgID,
		BuyerBranchID:  buyerBranchID,
		AllowedWorkIDs: []int64{workConnectedID, workUnconnID},
		OnlyInStock:    true,
		Limit:          1,
		Offset:         0,
	})
	if err != nil {
		t.Fatalf("ListBuyerOffers paged failed: %v", err)
	}
	if pagedTotal < 2 {
		t.Errorf("expected at least 2 offers (A and B) when both works allowed, got %d", pagedTotal)
	}
	if len(pagedOffers) != 1 {
		t.Errorf("expected exactly 1 offer on limit=1, got %d", len(pagedOffers))
	}
}

// Coverage belongs in the query, not in a pass over its result.
//
// The regression this guards is the one the offer-level pagination was built to
// remove and then re-introduced one layer down: the buying catalogue counted
// every institutionally-connected offer and then dropped the ones whose
// supplier does not deliver to the buyer's branch, so a page of 24 rendered two
// cards under a pager claiming 1,695 items. A count the page cannot fill is the
// defect, whichever predicate is missing from it.
func TestListBuyerOffers_CoverageFiltersTheCount(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	repo := postgres.NewRepository(db)

	base := catalog.BuyerOfferQuery{
		OnlyInStock: true,
		Limit:       10,
	}

	// Without coverage applied, the query is the unfiltered marketplace.
	_, unfilteredTotal, err := repo.ListBuyerOffers(ctx, base)
	if err != nil {
		t.Fatalf("ListBuyerOffers without coverage: %v", err)
	}

	// ApplyCoverage with nobody covering is an empty result, not an unfiltered
	// one. Reading a nil slice as "no filter" is what let uncovered suppliers
	// into the count.
	noneOffers, noneTotal, err := repo.ListBuyerOffers(ctx, func() catalog.BuyerOfferQuery {
		q := base
		q.ApplyCoverage = true
		q.CoveredVendorOrgIDs = nil
		return q
	}())
	if err != nil {
		t.Fatalf("ListBuyerOffers with empty coverage set: %v", err)
	}
	if noneTotal != 0 || len(noneOffers) != 0 {
		t.Errorf("no covering supplier must yield an empty page, got total=%d rows=%d",
			noneTotal, len(noneOffers))
	}

	if unfilteredTotal == 0 {
		return // nothing sellable on this database; the assertions below need rows
	}

	// A covering set that names a supplier with no offers must also narrow the
	// count, rather than being ignored because it is "just a filter".
	_, narrowedTotal, err := repo.ListBuyerOffers(ctx, func() catalog.BuyerOfferQuery {
		q := base
		q.ApplyCoverage = true
		q.CoveredVendorOrgIDs = []int64{-1}
		return q
	}())
	if err != nil {
		t.Fatalf("ListBuyerOffers with a non-existent covering supplier: %v", err)
	}
	if narrowedTotal != 0 {
		t.Errorf("coverage set naming no real supplier must count zero, got %d", narrowedTotal)
	}
}
