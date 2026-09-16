package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// bulkTestSKU namespaces every row these tests write, so cleanup removes exactly
// what was created and nothing else.
const bulkTestSKU = "BULKTEST"

// createAll lets these tests exercise brand and category creation. The toggles
// that govern it in production are covered by the service tests.
var createAll = catalog.BulkWriteOptions{CreateBrands: true, CreateCategories: true}

// bulkTestName namespaces product names too.
//
// Name matching is one of the behaviours under test, and it runs against
// whatever catalogue the test database holds — which on a shared database is a
// real one with thousands of products. A fixture named "بانادول اكسترا" matches
// a real row and turns an insert into an update, so the names carry a marker no
// real product has.
func bulkTestName(name string) string { return "BULKTEST " + name }

func newBulkRepo(t *testing.T) (*postgres.Repository, *database.DB) {
	t.Helper()
	db := getTestDB(t)

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.products WHERE sku LIKE $1`, bulkTestSKU+"%")
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE name->>'ar' LIKE 'BulkTest%'`)
			return nil
		})
	})

	return postgres.NewRepository(db), db
}

func bulkProduct(nameAR, sku string, price money.Amount) *catalog.Product {
	return &catalog.Product{
		Name:                 i18n.New(nameAR, ""),
		SKU:                  sku,
		Price:                price,
		InstitutionalWorkIDs: []int64{},
	}
}

func countBulkProducts(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT count(*) FROM catalog.products WHERE sku LIKE $1`, bulkTestSKU+"%").Scan(&n)
	})
	if err != nil {
		t.Fatalf("count products: %v", err)
	}
	return n
}

// The regression this exists for: the bulk INSERT bound institutional_work_ids
// through COALESCE($23, '{}'), which PostgreSQL resolves to text, so every row
// of every import failed with `column "institutional_work_ids" is of type
// bigint[] but expression is of type text`. Only a real INSERT against the real
// column types catches it.
func TestBulkUpsertProductsInsertsAgainstRealColumnTypes(t *testing.T) {
	repo, db := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	res, err := repo.BulkUpsertProducts(ctx, []*catalog.Product{
		bulkProduct(bulkTestName("بانادول اكسترا"), bulkTestSKU+"-1", money.MustParse("55.00")),
		bulkProduct(bulkTestName("كتافلام أقراص"), bulkTestSKU+"-2", money.MustParse("42.00")),
	}, createAll)
	if err != nil {
		t.Fatalf("bulk upsert failed: %v", err)
	}
	if res.Inserted != 2 || res.Updated != 0 {
		t.Fatalf("inserted=%d updated=%d, want 2 and 0", res.Inserted, res.Updated)
	}
	if n := countBulkProducts(t, db); n != 2 {
		t.Fatalf("catalogue holds %d rows, want 2", n)
	}
}

func TestBulkUpsertProductsMatchesExistingBySKU(t *testing.T) {
	repo, db := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	first := []*catalog.Product{bulkProduct(bulkTestName("بانادول اكسترا"), bulkTestSKU+"-1", money.MustParse("55.00"))}
	if _, err := repo.BulkUpsertProducts(ctx, first, createAll); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	second := []*catalog.Product{bulkProduct(bulkTestName("بانادول اكسترا"), bulkTestSKU+"-1", money.MustParse("60.00"))}
	res, err := repo.BulkUpsertProducts(ctx, second, createAll)
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if res.Inserted != 0 || res.Updated != 1 {
		t.Fatalf("inserted=%d updated=%d, want 0 and 1", res.Inserted, res.Updated)
	}
	if res.Matches[0] != catalog.MatchSKU {
		t.Errorf("match reason = %q, want %q", res.Matches[0], catalog.MatchSKU)
	}
	if n := countBulkProducts(t, db); n != 1 {
		t.Fatalf("catalogue holds %d rows, want 1", n)
	}
}

func TestBulkUpsertProductsMatchesExistingByFoldedName(t *testing.T) {
	repo, db := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	// No SKU on either write, and the names differ only by hamza and
	// ta-marbuta. Folding on both sides is what keeps this one product.
	if _, err := repo.BulkUpsertProducts(ctx, []*catalog.Product{
		{Name: i18n.New(bulkTestName("أوجمنتين حقنة"), ""), SKU: bulkTestSKU + "-N1", InstitutionalWorkIDs: []int64{}},
	}, createAll); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	res, err := repo.BulkUpsertProducts(ctx, []*catalog.Product{
		{Name: i18n.New(bulkTestName("اوجمنتين حقنه"), ""), InstitutionalWorkIDs: []int64{}},
	}, createAll)
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated=%d, want 1 (the names fold to the same product)", res.Updated)
	}
	if res.Matches[0] != catalog.MatchName {
		t.Errorf("match reason = %q, want %q", res.Matches[0], catalog.MatchName)
	}
	if n := countBulkProducts(t, db); n != 1 {
		t.Fatalf("catalogue holds %d rows, want 1", n)
	}
}

// A statement PostgreSQL refuses aborts the transaction, after which every later
// statement in the batch reports the same "transaction is aborted" error. This
// covers the savepoint replay that recovers the real culprit from that.
func TestBulkUpsertProductsNamesTheRowThatFailed(t *testing.T) {
	repo, db := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	// Beyond NUMERIC(12,2). The service rejects this at parse time in normal
	// operation; here it exercises the repository's diagnosis directly.
	res, err := repo.BulkUpsertProducts(ctx, []*catalog.Product{
		bulkProduct(bulkTestName("صنف سليم أول"), bulkTestSKU+"-1", money.MustParse("10.00")),
		bulkProduct(bulkTestName("صنف بسعر خارج النطاق"), bulkTestSKU+"-2", money.FromMinor(99999999999999)),
		bulkProduct(bulkTestName("صنف سليم ثالث"), bulkTestSKU+"-3", money.MustParse("30.00")),
	}, createAll)

	if err == nil {
		t.Fatal("expected the write to fail")
	}
	if len(res.Failures) != 1 {
		t.Fatalf("got %d named failures, want 1: %+v", len(res.Failures), res.Failures)
	}
	if res.Failures[0].Index != 1 {
		t.Errorf("failure index = %d, want 1 (the offending row, not the first)", res.Failures[0].Index)
	}
	if !strings.Contains(res.Failures[0].Name, "خارج النطاق") {
		t.Errorf("failure names %q, want the offending product", res.Failures[0].Name)
	}
	if !strings.Contains(err.Error(), "خارج النطاق") {
		t.Errorf("error message %q does not name the offending row", err.Error())
	}

	// All or nothing: the two good rows must not survive a failed import.
	if n := countBulkProducts(t, db); n != 0 {
		t.Fatalf("catalogue holds %d rows after a failed import, want 0", n)
	}
	if res.Inserted != 0 || res.Updated != 0 {
		t.Errorf("reported inserted=%d updated=%d after a rollback, want 0 and 0", res.Inserted, res.Updated)
	}
}

func TestBulkUpsertProductsRegistersNewManufacturers(t *testing.T) {
	repo, _ := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	prods := []*catalog.Product{
		bulkProduct(bulkTestName("بانادول اكسترا"), bulkTestSKU+"-1", money.MustParse("55.00")),
		bulkProduct(bulkTestName("بانادول نايت"), bulkTestSKU+"-2", money.MustParse("65.00")),
	}
	prods[0].ManufacturingCompanies = "BulkTest Pharma"
	// Same company, spelled with different casing and spacing: one brand, not two.
	prods[1].ManufacturingCompanies = "bulktest  pharma"

	res, err := repo.BulkUpsertProducts(ctx, prods, createAll)
	if err != nil {
		t.Fatalf("bulk upsert failed: %v", err)
	}
	if res.BrandsCreated != 1 {
		t.Errorf("created %d brands, want 1", res.BrandsCreated)
	}
	if prods[0].BrandID == nil || prods[1].BrandID == nil {
		t.Fatal("a product was left without a brand")
	}
	if *prods[0].BrandID != *prods[1].BrandID {
		t.Errorf("brand ids %d and %d differ for one manufacturer", *prods[0].BrandID, *prods[1].BrandID)
	}
}
