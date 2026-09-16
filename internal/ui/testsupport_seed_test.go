package ui_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func seedOrg(t *testing.T, db *database.DB, orgType string) int64 {
	t.Helper()
	ctx := context.Background()
	var orgID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (
				name, type, status, created_at, updated_at
			) VALUES (
				'{"ar": "مؤسسة اختبارية", "en": "Test Org"}'::jsonb,
				$1, 'approved', now(), now()
			) RETURNING id
		`, orgType).Scan(&orgID)
	})
	if err != nil {
		t.Fatalf("seedOrg failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM org.organizations WHERE id = $1", orgID)
			return nil
		})
	})

	return orgID
}

func seedBranch(t *testing.T, db *database.DB, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var branchID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO org.branches (
				organization_id, name, address, latitude, longitude, is_main, status, created_at, updated_at
			) VALUES (
				$1, '{"ar": "فرع تجريبي", "en": "Test Branch"}'::jsonb,
				'شارع التحرير، القاهرة', 30.0444, 31.2357, true, 'active', now(), now()
			) RETURNING id
		`, orgID).Scan(&branchID)
	})
	if err != nil {
		t.Fatalf("seedBranch failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM org.branches WHERE id = $1", branchID)
			return nil
		})
	})

	return branchID
}

func seedUser(t *testing.T, db *database.DB, orgID int64, role string) int64 {
	t.Helper()
	ctx := context.Background()
	var userID int64

	if role == "" || (role != "user" && role != "support" && role != "admin" && role != "super_admin" && role != "developer") {
		role = "user"
	}

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		email := fmt.Sprintf("test_user_%d_%d@dawa24.test", time.Now().UnixNano(), orgID)
		return tx.QueryRow(txCtx, `
			INSERT INTO identity.users (
				name, email, password_hash, role, status, created_at, updated_at
			) VALUES (
				'{"ar":"مستخدم اختبار"}'::jsonb, $1, 'hashed_pw', $2, 'active', now(), now()
			) RETURNING id
		`, email, role).Scan(&userID)
	})
	if err != nil {
		t.Fatalf("seedUser failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM identity.users WHERE id = $1", userID)
			return nil
		})
	})

	return userID
}

func seedProduct(t *testing.T, db *database.DB) int64 {
	t.Helper()
	ctx := context.Background()
	var prodID int64
	orgID := seedOrg(t, db, "vendor")

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.products (
				organization_id, name, status, created_at, updated_at
			) VALUES (
				$1, '{"ar": "بنادول إكسترا", "en": "Panadol Extra"}'::jsonb,
				'active', now(), now()
			) RETURNING id
		`, orgID).Scan(&prodID)
	})
	if err != nil {
		t.Fatalf("seedProduct failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM catalog.products WHERE id = $1", prodID)
			return nil
		})
	})

	return prodID
}

func seedOffer(t *testing.T, db *database.DB, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var offerID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO promo.offers (
				organization_id, title, discount_type, discount_value, min_order_amount, starts_at, expires_at, is_active, created_at, updated_at
			) VALUES (
				$1, '{"ar": "عرض خصم خاص", "en": "Special Offer"}'::jsonb,
				'percentage', 15.00, 100.00, now(), now() + interval '30 days', true, now(), now()
			) RETURNING id
		`, orgID).Scan(&offerID)
	})
	if err != nil {
		t.Fatalf("seedOffer failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM promo.offers WHERE id = $1", offerID)
			return nil
		})
	})

	return offerID
}

func seedOrder(t *testing.T, db *database.DB, customerOrgID, vendorOrgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var orderID int64
	userID := seedUser(t, db, customerOrgID, "user")

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		orderNum := fmt.Sprintf("ORD-%d", time.Now().UnixNano()%10000000)
		return tx.QueryRow(txCtx, `
			INSERT INTO commerce.orders (
				order_number, customer_id, organization_id, total_amount, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 500.00, 'pending', now(), now()
			) RETURNING id
		`, orderNum, userID, customerOrgID).Scan(&orderID)
	})
	if err != nil {
		t.Fatalf("seedOrder failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM commerce.orders WHERE id = $1", orderID)
			return nil
		})
	})

	return orderID
}

func seedInvoice(t *testing.T, db *database.DB, orgID int64, amount money.Amount) int64 {
	t.Helper()
	ctx := context.Background()
	var invID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		invNum := fmt.Sprintf("INV-%d", time.Now().UnixNano()%10000000)
		return tx.QueryRow(txCtx, `
			INSERT INTO billing.invoices (
				invoice_number, organization_id, total_amount, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 'unpaid', now(), now()
			) RETURNING id
		`, invNum, orgID, amount).Scan(&invID)
	})
	if err != nil {
		t.Fatalf("seedInvoice failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM billing.invoices WHERE id = $1", invID)
			return nil
		})
	})

	return invID
}

func seedPayment(t *testing.T, db *database.DB, orgID int64, amount money.Amount, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var payID int64

	userID := seedUser(t, db, orgID, "member")

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO billing.payments (
				user_id, organization_id, amount, method, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 'cash', $4, now(), now()
			) RETURNING id
		`, userID, orgID, amount, status).Scan(&payID)
	})
	if err != nil {
		t.Fatalf("seedPayment failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM billing.payments WHERE id = $1", payID)
			return nil
		})
	})

	return payID
}

func seedWarehouse(t *testing.T, db *database.DB, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var whID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO inventory.warehouses (
				organization_id, name, is_active, created_at, updated_at
			) VALUES (
				$1, '{"ar": "مستودع تجريبي", "en": "Test Warehouse"}'::jsonb,
				true, now(), now()
			) RETURNING id
		`, orgID).Scan(&whID)
	})
	if err != nil {
		t.Fatalf("seedWarehouse failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM inventory.warehouses WHERE id = $1", whID)
			return nil
		})
	})

	return whID
}

func seedPolicy(t *testing.T, db *database.DB, key, title, content string) int64 {
	t.Helper()
	ctx := context.Background()
	var policyID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO platform_admin.policies (
				policy_key, title, content, is_published, version, created_at, updated_at
			) VALUES (
				$1, jsonb_build_object('ar', $2, 'en', $2), jsonb_build_object('ar', $3, 'en', $3), true, '1.0', now(), now()
			) RETURNING id
		`, key, title, content).Scan(&policyID)
	})
	if err != nil {
		t.Fatalf("seedPolicy failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM platform_admin.policies WHERE id = $1", policyID)
			return nil
		})
	})

	return policyID
}
