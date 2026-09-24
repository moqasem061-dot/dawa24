package ui_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
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
