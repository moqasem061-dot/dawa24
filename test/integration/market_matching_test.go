package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	comparepg "github.com/moqasem061-dot/dawa24/internal/modules/compare/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestMarketMatchingSearchAndAtomicReplacement(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(db.Close)
	ctx := database.AsSystem(context.Background())
	repo := comparepg.NewRepository(db)
	var userID, productID, organizationID int64
	err := db.InTx(ctx, func(c context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO identity.users(email,password_hash) VALUES(gen_random_uuid()::text || '@market.test','test-only') RETURNING id`).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(c, `INSERT INTO org.organizations(name) VALUES('{"ar":"اختبار السوق","en":"Market test"}') RETURNING id`).Scan(&organizationID); err != nil {
			return err
		}
		return tx.QueryRow(c, `INSERT INTO catalog.products(organization_id,name) VALUES($1,'{"ar":"بانادول 500 مجم","en":"Panadol 500 mg"}') RETURNING id`, organizationID).Scan(&productID)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(ctx, func(c context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(c, `DELETE FROM compare.files WHERE user_id=$1`, userID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(c, `DELETE FROM catalog.products WHERE id=$1`, productID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(c, `DELETE FROM org.organizations WHERE id=$1`, organizationID); err != nil {
				return err
			}
			_, err = tx.Exec(c, `DELETE FROM identity.users WHERE id=$1`, userID)
			return err
		})
	})
	file := &compare.CompareFile{UserID: userID, IsTempWarehouse: true, OriginalFilename: "warehouse.csv", SupplierName: "market regression", Status: compare.FileProcessing}
	if err := repo.CreateFile(ctx, file); err != nil {
		t.Fatal(err)
	}
	row := &compare.CompareFileRow{FileID: file.ID, RowNumber: 1, RawName: "pnd warehouse name", Price: money.MustParse("123.45"), Discount: 10, PriceAfterDiscount: money.MustParse("111.10"), MatchedProductID: &productID, MatchState: "accepted", MatchMethod: compare.MatchMethodExact, MatchDecision: json.RawMessage(`{"v":1,"state":"accepted"}`)}
	if err := repo.ReplaceFileRows(ctx, file, []*compare.CompareFileRow{row}); err != nil {
		t.Fatal(err)
	}
	// An unrelated, high-discount private upload must never enter the public board.
	private := &compare.CompareFile{UserID: userID, OriginalFilename: "private.csv", SupplierName: "private", Status: compare.FileReady}
	if err := repo.CreateFile(ctx, private); err != nil {
		t.Fatal(err)
	}
	privateRow := *row
	privateRow.FileID = private.ID
	privateRow.RawName = "Panadol private secret"
	privateRow.Discount = 99
	if err := repo.InsertFileRows(ctx, []*compare.CompareFileRow{&privateRow}); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []compare.MarketDiscountsFilter{{Query: "بانادول"}, {Query: "500 Panadol"}, {Query: "pnd"}, {Query: "typo", CatalogIDs: []int64{productID}}, {Query: "PANADOL", SortBy: "relevance"}} {
		result, err := repo.ListMarketDiscounts(ctx, filter)
		if err != nil {
			t.Fatalf("query %+v: %v", filter, err)
		}
		if len(result.Items) != 1 || result.Items[0].FileID != file.ID || result.Items[0].CatalogNameEN != "Panadol 500 mg" || result.Items[0].ProductName != row.RawName {
			t.Fatalf("query %+v: %+v", filter, result.Items)
		}
		count, err := repo.CountMarketDiscounts(ctx, filter)
		if err != nil || count != 1 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
	for _, q := range []string{"%", "_", "x' OR 1=1 --", "nonexistent drug"} {
		result, err := repo.ListMarketDiscounts(ctx, compare.MarketDiscountsFilter{Query: q})
		if err != nil || len(result.Items) != 0 {
			t.Fatalf("literal query %q: %+v %v", q, result, err)
		}
	}
	// COPY failure must roll back deletion and keep the original matching decision.
	broken := *row
	broken.RowNumber = 2
	broken.MatchState = "invalid-state"
	if err := repo.ReplaceFileRows(ctx, file, []*compare.CompareFileRow{&broken}); err == nil {
		t.Fatal("invalid state unexpectedly inserted")
	}
	rows, err := repo.ListFileRows(ctx, file.ID, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].RawName != row.RawName {
		t.Fatalf("rollback lost original row: %v %+v", err, rows)
	}
	stale := *file
	if err := repo.RenameFile(ctx, file.ID, "changed during matching"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceFileRows(ctx, &stale, []*compare.CompareFileRow{row}); err == nil {
		t.Fatal("stale replacement overwrote a newer upload")
	}
	var state string
	err = db.InReadTx(ctx, func(c context.Context, tx pgx.Tx) error {
		return tx.QueryRow(c, `SELECT match_state FROM compare.file_rows WHERE file_id=$1`, file.ID).Scan(&state)
	})
	if err != nil || state != "accepted" {
		t.Fatalf("canonical state lost: %q %v", state, err)
	}
	newer := &compare.CompareFile{UserID: userID, IsTempWarehouse: true, OriginalFilename: "newer.csv", SupplierName: "newest warehouse", Status: compare.FileReady}
	if err := repo.CreateFile(ctx, newer); err != nil {
		t.Fatal(err)
	}
	newRow := *row
	newRow.FileID = newer.ID
	newRow.Discount = 1
	newRow.RawName = "Panadol recent offer"
	if err := repo.InsertFileRows(ctx, []*compare.CompareFileRow{&newRow}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "Panadol"} {
		result, err := repo.ListMarketDiscounts(ctx, compare.MarketDiscountsFilter{Query: query})
		if err != nil || len(result.Items) != 2 || result.Items[0].FileID != newer.ID {
			t.Fatalf("newest search ordering: err=%v result=%+v", err, result)
		}
	}
}
