package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Every path that reads a staged row must read the same columns.
//
// The regression this exists for: the listing and single-row queries grew a
// join for the matched product's name and the commit path did not. Nothing
// caught it, because the commit path is the one read an ordinary test never
// reaches — so it surfaced in production as "number of field descriptions must
// equal number of destinations, got 11 and 13", raised at the moment an admin
// pressed save on a reviewed import.
//
// The three reads now share one column list, which makes the mismatch
// impossible to reintroduce. This test is the belt to that braces: it walks all
// three against the real database, which is the only place a column count is
// actually checked.
func TestEveryStagingReadPathScansTheSameColumns(t *testing.T) {
	repo, db := newBulkRepo(t)
	ctx := database.AsSystem(context.Background())

	// A real catalogue product for the row to be matched against, so the join
	// in the shared column list actually has something to return.
	matched := bulkProduct(bulkTestName("ستاجينج"), bulkTestSKU+"-STAGE-1", money.FromMinor(1000))
	if _, err := repo.BulkUpsertProducts(ctx, []*catalog.Product{matched}, createAll); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if matched.ID == 0 {
		t.Fatal("seed product has no id")
	}

	session := &catalog.ImportSession{
		OrganizationID: matched.OrganizationID,
		Filename:       "staging-columns.csv",
		Status:         catalog.SessionReady,
		Mode:           catalog.ModeUpdateAndAdd,
		Options:        catalog.DefaultImportOptions(),
	}
	if err := repo.CreateImportSession(ctx, session, []byte("x")); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, `DELETE FROM catalog.import_sessions WHERE id = $1`, session.ID)
			return nil
		})
	})

	staged := []*catalog.StagingRow{{
		SourceRow:        2,
		Action:           catalog.ActionUpdate,
		Included:         true,
		MatchedProductID: &matched.ID,
		MatchReason:      catalog.MatchSimilar,
		Product: &catalog.Product{
			Name:                 i18n.New(bulkTestName("ستاجينج"), ""),
			SKU:                  bulkTestSKU + "-STAGE-1",
			Price:                money.FromMinor(1200),
			InstitutionalWorkIDs: []int64{},
		},
	}}
	if err := repo.ReplaceStagingRows(ctx, session.ID, staged); err != nil {
		t.Fatalf("stage rows: %v", err)
	}

	// Path one: the review table.
	listed, total, err := repo.ListStagingRows(ctx, session.ID, catalog.StagingFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list staging rows: %v", err)
	}
	if total != 1 || len(listed) != 1 {
		t.Fatalf("listed %d rows (total %d), want 1", len(listed), total)
	}
	if listed[0].MatchedProductName == "" {
		t.Error("listing did not resolve the matched product name")
	}

	// Path two: one row, as the review screen re-renders it after an edit.
	one, err := repo.GetStagingRow(ctx, session.ID, listed[0].ID)
	if err != nil {
		t.Fatalf("get staging row: %v", err)
	}
	if one.MatchedProductName != listed[0].MatchedProductName {
		t.Errorf("single read disagrees with the listing: %q vs %q",
			one.MatchedProductName, listed[0].MatchedProductName)
	}

	// Path three: the commit. This is the one that was broken, and the one an
	// admin only reaches after reviewing the whole file.
	committable, err := repo.LoadCommittableRows(ctx, session.ID)
	if err != nil {
		t.Fatalf("load committable rows: %v", err)
	}
	if len(committable) != 1 {
		t.Fatalf("loaded %d committable rows, want 1", len(committable))
	}
	if committable[0].MatchedProductID == nil || *committable[0].MatchedProductID != matched.ID {
		t.Error("commit path lost the matched product id")
	}
	if committable[0].Product == nil || committable[0].Product.SKU != bulkTestSKU+"-STAGE-1" {
		t.Error("commit path lost the staged payload")
	}
}
