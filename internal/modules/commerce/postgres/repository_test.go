package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v; skipping", err)
	}

	migrations, _ := database.LoadMigrations(dbfs.Migrations, "migrations")
	if pending, _ := db.PendingCount(ctx, migrations); pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}

	return db
}

const (
	testCommerceUserID   int64 = 88390
	testCommerceVendorID int64 = 88391
	testCommerceCustID   int64 = 88392
	testCommerceProdID   int64 = 88393
	testCommerceVarID    int64 = 88394
	testCommerceCatID    int64 = 88395
	testCommerceBrandID  int64 = 88396
)

func resetCommerceFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.order_status_history WHERE changed_by_user_id = $1`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete order_status_history: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.order_lines WHERE organization_id = $1`, testCommerceVendorID); err != nil {
			return fmt.Errorf("delete order_lines: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.order_shipments WHERE organization_id = $1`, testCommerceVendorID); err != nil {
			return fmt.Errorf("delete order_shipments: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.orders WHERE customer_id = $1`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete orders: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.wishlists WHERE user_id = $1`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete wishlists: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.quote_requests WHERE organization_id IN ($1, $2)`, testCommerceVendorID, testCommerceCustID); err != nil {
			return fmt.Errorf("delete quote_requests: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.cart_items WHERE cart_id IN (SELECT id FROM commerce.carts WHERE user_id = $1)`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete cart_items: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM commerce.carts WHERE user_id = $1`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete carts: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.product_variants WHERE id = $1`, testCommerceVarID); err != nil {
			return fmt.Errorf("delete product_variants: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.products WHERE id = $1`, testCommerceProdID); err != nil {
			return fmt.Errorf("delete products: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.categories WHERE id = $1`, testCommerceCatID); err != nil {
			return fmt.Errorf("delete categories: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE id = $1`, testCommerceBrandID); err != nil {
			return fmt.Errorf("delete brands: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id IN ($1, $2)`, testCommerceVendorID, testCommerceCustID); err != nil {
			return fmt.Errorf("delete organizations: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testCommerceUserID); err != nil {
			return fmt.Errorf("delete users: %w", err)
		}

		// Setup prerequisite user, orgs, and product
		if _, err := tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name)
			VALUES ($1, 'commuser88390@example.com', 'x', '{"ar":"مستخدم","en":"Commerce User"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email`, testCommerceUserID); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"بائع","en":"Commerce Vendor Org"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testCommerceVendorID); err != nil {
			return fmt.Errorf("insert vendor org: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"عميل","en":"Commerce Cust Org"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testCommerceCustID); err != nil {
			return fmt.Errorf("insert cust org: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO catalog.categories (id, name)
			VALUES ($1, '{"ar":"قسم","en":"Comm Cat"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testCommerceCatID); err != nil {
			return fmt.Errorf("insert category: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO catalog.brands (id, name)
			VALUES ($1, '{"ar":"ماركة","en":"Comm Brand"}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`, testCommerceBrandID); err != nil {
			return fmt.Errorf("insert brand: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO catalog.products (id, organization_id, category_id, brand_id, name, dosage_form)
			VALUES ($1, $2, $3, $4, '{"ar":"منتج","en":"Comm Prod"}'::jsonb, 'tablet')
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`,
			testCommerceProdID, testCommerceVendorID, testCommerceCatID, testCommerceBrandID); err != nil {
			return fmt.Errorf("insert product: %w", err)
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO catalog.product_variants (id, organization_id, product_id, name, sku, price)
			VALUES ($1, $2, $3, '{"ar":"عبوة","en":"Commerce Variant"}'::jsonb, 'COMM-SKU-1', 100.00)
			ON CONFLICT (id) DO UPDATE SET price = EXCLUDED.price`,
			testCommerceVarID, testCommerceVendorID, testCommerceProdID); err != nil {
			return fmt.Errorf("insert variant: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestCommerceRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	resetCommerceFixtures(t, db)

	ctx := context.Background()
	repo := postgres.NewRepository(db)

	t.Run("Cart_Operations", func(t *testing.T) {
		cart, err := repo.GetOrCreateCart(ctx, testCommerceUserID)
		if err != nil {
			t.Fatalf("GetOrCreateCart failed: %v", err)
		}

		item := &commerce.CartItem{
			CartID:           cart.ID,
			ProductID:        testCommerceProdID,
			ProductVariantID: testCommerceVarID,
			Quantity:         2,
			UnitPrice:        money.MustParse("100.00"),
		}
		err = repo.AddToCartItem(ctx, cart.ID, item)
		if err != nil {
			t.Fatalf("AddToCartItem failed: %v", err)
		}

		cartWithItems, err := repo.GetCartWithItems(ctx, cart.ID)
		if err != nil {
			t.Fatalf("GetCartWithItems failed: %v", err)
		}
		if len(cartWithItems.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(cartWithItems.Items))
		}

		err = repo.SetCartItemQuantity(ctx, cart.ID, testCommerceVarID, 5)
		if err != nil {
			t.Fatalf("SetCartItemQuantity failed: %v", err)
		}

		err = repo.RemoveCartItem(ctx, cart.ID, testCommerceVarID)
		if err != nil {
			t.Fatalf("RemoveCartItem failed: %v", err)
		}

		err = repo.ClearCart(ctx, cart.ID)
		if err != nil {
			t.Fatalf("ClearCart failed: %v", err)
		}
	})

	t.Run("Wishlist_Operations", func(t *testing.T) {
		err := repo.AddToWishlist(ctx, testCommerceUserID, testCommerceProdID)
		if err != nil {
			t.Fatalf("AddToWishlist failed: %v", err)
		}

		list, err := repo.ListWishlist(ctx, testCommerceUserID)
		if err != nil || len(list) == 0 {
			t.Fatalf("ListWishlist failed: %v", err)
		}

		err = repo.RemoveFromWishlist(ctx, testCommerceUserID, testCommerceProdID)
		if err != nil {
			t.Fatalf("RemoveFromWishlist failed: %v", err)
		}
	})

	t.Run("Quotes_CRUD", func(t *testing.T) {
		pID := testCommerceProdID
		qr := &commerce.QuoteRequest{
			OrganizationID:    testCommerceVendorID,
			CustomerOrgID:     testCommerceCustID,
			ProductID:         &pID,
			ProductName:       "Comm Prod",
			RequestedQuantity: 500,
			TargetUnitPrice:   money.MustParse("90.00"),
			Status:            commerce.QuotePending,
			BuyerNotes:        "Quote for 500 units",
		}
		err := repo.CreateQuoteRequest(ctx, qr)
		if err != nil {
			t.Fatalf("CreateQuoteRequest failed: %v", err)
		}
		if qr.ID == 0 {
			t.Fatal("expected generated quote request ID")
		}

		gotQuote, err := repo.GetQuoteRequestByID(ctx, qr.ID)
		if err != nil {
			t.Fatalf("GetQuoteRequestByID failed: %v", err)
		}
		if gotQuote.BuyerNotes != "Quote for 500 units" {
			t.Errorf("got %q, want 'Quote for 500 units'", gotQuote.BuyerNotes)
		}

		err = repo.UpdateQuoteStatus(ctx, qr.ID, commerce.QuoteQuoted, money.MustParse("95.00"), "Discount applied")
		if err != nil {
			t.Fatalf("UpdateQuoteStatus failed: %v", err)
		}

		list, err := repo.ListQuoteRequestsByOrg(ctx, testCommerceCustID, false, 10, 0)
		if err != nil || len(list) == 0 {
			t.Fatalf("ListQuoteRequestsByOrg failed: %v", err)
		}
	})
}
