package integration_test

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	comparepg "github.com/moqasem061-dot/dawa24/internal/modules/compare/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The market board rebuild (2026-11-14) against a real database: the product
// filter, the COUNT+MAX scan whose maximum is board-wide, and the search box's
// grouped suggestions. A private compare upload that matches the same words is
// in the fixture the whole time, and must appear in none of it.
func TestMarketBoardProductFilterMaxDiscountAndSuggestions(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(db.Close)
	ctx := database.AsSystem(context.Background())
	repo := comparepg.NewRepository(db)

	token := fmt.Sprintf("zq%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(1_000_000_000))
	var userID, orgID, productA, productB int64
	if err := db.InTx(ctx, func(c context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(c, `INSERT INTO identity.users(email,password_hash) VALUES(gen_random_uuid()::text || '@board.test','test-only') RETURNING id`).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(c, `INSERT INTO org.organizations(name) VALUES('{"ar":"اختبار اللوحة","en":"Board test"}') RETURNING id`).Scan(&orgID); err != nil {
			return err
		}
		if err := tx.QueryRow(c, `INSERT INTO catalog.products(organization_id,name) VALUES($1, jsonb_build_object('ar','بانادول '||$2::text,'en','Panadol '||$2::text)) RETURNING id`, orgID, token).Scan(&productA); err != nil {
			return err
		}
		return tx.QueryRow(c, `INSERT INTO catalog.products(organization_id,name) VALUES($1, jsonb_build_object('ar','كولد '||$2::text,'en','Cold '||$2::text)) RETURNING id`, orgID, token).Scan(&productB)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(ctx, func(c context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(c, `DELETE FROM compare.files WHERE user_id=$1`, userID); err != nil {
				return err
			}
			if _, err := tx.Exec(c, `DELETE FROM catalog.products WHERE id = ANY($1)`, []int64{productA, productB}); err != nil {
				return err
			}
			if _, err := tx.Exec(c, `DELETE FROM org.organizations WHERE id=$1`, orgID); err != nil {
				return err
			}
			_, err := tx.Exec(c, `DELETE FROM identity.users WHERE id=$1`, userID)
			return err
		})
	})

	newFile := func(name string, warehouse bool, status compare.CompareFileStatus) *compare.CompareFile {
		f := &compare.CompareFile{UserID: userID, IsTempWarehouse: warehouse, OriginalFilename: name + ".csv", SupplierName: name, Status: status}
		if err := repo.CreateFile(ctx, f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	w1 := newFile("wh1-"+token, true, compare.FileReady)
	w2 := newFile("wh2-"+token, true, compare.FileReady)
	private := newFile("private-"+token, false, compare.FileReady)

	row := func(f *compare.CompareFile, n int, name, price string, disc float64, product *int64) *compare.CompareFileRow {
		state := ""
		if product != nil {
			state = "accepted"
		}
		return &compare.CompareFileRow{FileID: f.ID, RowNumber: n, RawName: name, Price: money.MustParse(price), Discount: disc,
			PriceAfterDiscount: money.MustParse(price), MatchedProductID: product, MatchState: state}
	}
	if err := repo.ReplaceFileRows(ctx, w1, []*compare.CompareFileRow{
		row(w1, 1, "panadol "+token+" 500 w1", "100.00", 10, &productA),
		row(w1, 2, "unmatched "+token+" strips", "20.00", 7, nil),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceFileRows(ctx, w2, []*compare.CompareFileRow{
		row(w2, 1, "panadol "+token+" 500 w2", "90.00", 63, &productA),
		row(w2, 2, "cold "+token+" syrup", "50.00", 5, &productB),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertFileRows(ctx, []*compare.CompareFileRow{
		row(private, 1, "panadol "+token+" private secret", "10.00", 99, &productA),
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("the board-wide maximum is read with the count and ignores a private upload", func(t *testing.T) {
		got, err := repo.CountMarketDiscounts(ctx, compare.MarketDiscountsFilter{Query: token})
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 4 || got.MaxDiscount != 63 {
			t.Fatalf("count = %+v, want 4 rows and a maximum of 63 (the private upload's 99 must not count)", got)
		}
		// The maximum follows the filter, not the page: narrowing to the 10 %
		// warehouse line gives that line's discount.
		narrowed, err := repo.CountMarketDiscounts(ctx, compare.MarketDiscountsFilter{Query: token, Supplier: w1.SupplierName})
		if err != nil {
			t.Fatal(err)
		}
		if narrowed.Total != 2 || narrowed.MaxDiscount != 10 {
			t.Fatalf("narrowed count = %+v, want 2 rows and a maximum of 10", narrowed)
		}
	})

	t.Run("a product filter shows every warehouse's offer for it, and the same rows are counted", func(t *testing.T) {
		filter := compare.MarketDiscountsFilter{ProductID: &productA, SortBy: "discount_desc", Limit: 24}
		page, err := repo.ListMarketDiscounts(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 2 {
			t.Fatalf("items = %d, want the two warehouse offers for the product", len(page.Items))
		}
		for _, item := range page.Items {
			if item.MatchedProductID == nil || *item.MatchedProductID != productA {
				t.Fatalf("an offer for another product leaked in: %+v", item)
			}
			if strings.Contains(item.ProductName, "private") {
				t.Fatal("the private upload reached the board")
			}
		}
		if page.Items[0].DiscountPercent != 63 {
			t.Fatalf("first discount = %v, want 63 (highest first)", page.Items[0].DiscountPercent)
		}
		count, err := repo.CountMarketDiscounts(ctx, filter)
		if err != nil || count.Total != 2 || count.MaxDiscount != 63 {
			t.Fatalf("count = %+v err=%v, want 2 / 63", count, err)
		}
		ar, en, err := repo.MarketProductNames(ctx, productA)
		if err != nil || !strings.Contains(ar, token) || !strings.Contains(en, token) {
			t.Fatalf("names = %q / %q err=%v", ar, en, err)
		}
	})

	t.Run("suggestions are grouped, capped and never include a private upload", func(t *testing.T) {
		got, err := repo.SuggestMarketDiscounts(ctx, token, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("suggestions = %d (%+v), want product A, product B and the unmatched name", len(got), got)
		}
		var a *compare.MarketSuggestion
		unmatched := 0
		for i := range got {
			if strings.Contains(strings.ToLower(got[i].FileName), "private") {
				t.Fatal("a private compare upload was suggested")
			}
			if got[i].ProductID == nil {
				unmatched++
			} else if *got[i].ProductID == productA {
				a = &got[i]
			}
		}
		if unmatched != 1 || a == nil {
			t.Fatalf("grouping wrong: unmatched=%d productA=%v", unmatched, a)
		}
		if a.Offers != 2 || a.Warehouses != 2 || a.BestDiscount != 63 || a.LowestPrice.String() != "90.00" {
			t.Fatalf("product A line = %+v, want 2 offers, 2 warehouses, best 63, from 90.00", *a)
		}
		if a.NameAR == "" || a.NameEN == "" {
			t.Fatalf("catalogue names missing: %+v", *a)
		}
		capped, err := repo.SuggestMarketDiscounts(ctx, token, 2)
		if err != nil || len(capped) != 2 {
			t.Fatalf("capped = %d err=%v, want 2", len(capped), err)
		}
	})
}
