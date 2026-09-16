package jobs

import "strings"

import "testing"

// These tests guard one specific, silent failure.
//
// Before this change the reindex wrote `NULL::bigint AS variant_id` and
// `0 AS stock_quantity` into every row of catalog.product_index. The code
// compiled, the job succeeded, the index filled up, and every query of the form
// "products with stock_quantity > 0" returned nothing — for 28,786 rows, in
// production, undetected. There was no test that could have caught it because
// nothing asserted the projection carried real values.
//
// A database-backed test would be better and belongs in the repository suite.
// These run without a database, which means they run on every commit, and they
// fail the moment someone reintroduces a placeholder.

func TestVariantSelectProjectsRealVariantID(t *testing.T) {
	if strings.Contains(variantSelect, "NULL::bigint AS variant_id") {
		t.Fatal("variantSelect writes a literal NULL variant_id; vendor offers would be invisible in the read model")
	}
	if !strings.Contains(variantSelect, "v.id AS variant_id") {
		t.Fatal("variantSelect must project the variant's own id into variant_id")
	}
}

func TestVariantSelectProjectsRealStock(t *testing.T) {
	if strings.Contains(variantSelect, "0 AS stock_quantity") {
		t.Fatal("variantSelect writes a literal 0 stock_quantity; every availability query would return nothing")
	}
	if !strings.Contains(variantSelect, "COALESCE(st.qty, 0) AS stock_quantity") {
		t.Fatal("variantSelect must read stock from the inventory.stocks aggregate")
	}
	if !strings.Contains(stockJoin, "inventory.stocks") {
		t.Fatal("stock must come from inventory.stocks, the authoritative table")
	}
}

func TestParentSelectAggregatesVariantStock(t *testing.T) {
	if strings.Contains(parentSelect, "0 AS stock_quantity") {
		t.Fatal("parentSelect writes a literal 0 stock_quantity")
	}
	if !strings.Contains(parentSelect, "COALESCE(pstock.qty, 0) AS stock_quantity") {
		t.Fatal("a parent's stock must be the sum of its variants' stock")
	}
}

func TestBothRowTypesAreIndexed(t *testing.T) {
	if !strings.Contains(parentSelect, "'parent' AS product_type") {
		t.Fatal("parentSelect must tag its rows 'parent'")
	}
	if !strings.Contains(variantSelect, "'variant' AS product_type") {
		t.Fatal("variantSelect must tag its rows 'variant'")
	}
	// The read model is useless for "who sells this" without variant rows, and
	// the previous implementation had no variant path at all.
	if sweepVariants == "" || !strings.Contains(sweepVariants, "catalog.product_variants") {
		t.Fatal("the full sweep must index variants, not only parents")
	}
}

func TestSweepAndUpsertShareOneProjection(t *testing.T) {
	// Parent and variant projections must agree on how stock is counted, or the
	// two row types disagree about the same product. Sharing the constants is
	// what enforces that; this asserts the sharing has not been undone.
	if !strings.Contains(sweepVariants, variantSelect) {
		t.Fatal("sweepVariants must be built from variantSelect")
	}
	if !strings.Contains(upsertVariants, variantSelect) {
		t.Fatal("upsertVariants must be built from variantSelect")
	}
	if !strings.Contains(sweepParents, parentSelect) {
		t.Fatal("sweepParents must be built from parentSelect")
	}
	if !strings.Contains(upsertParent, parentSelect) {
		t.Fatal("upsertParent must be built from parentSelect")
	}
}

func TestStaleVariantsArePruned(t *testing.T) {
	// A deactivated offer that stays in the index keeps being sold.
	if !strings.Contains(pruneVariants, "DELETE FROM catalog.product_index") {
		t.Fatal("a single-product reindex must remove variants that are no longer active")
	}
	if !strings.Contains(pruneVariants, "product_type = 'variant'") {
		t.Fatal("pruning must be scoped to variant rows so parents are not deleted")
	}
}
