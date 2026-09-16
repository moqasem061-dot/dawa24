package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ensureAdminAccount checks if ADMIN_EMAIL is set (or defaults to admin@dawa24.net)
// and creates or updates the super_admin account.
func ensureAdminAccount(ctx context.Context, db *database.DB, log *slog.Logger) error {
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@dawa24.net"
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		password = "Admin!Dawa24!2026"
	}

	return createOrUpdateAdmin(ctx, db, log, email, password, "Platform Super Admin", "مدير النظام العام")
}

// createOrUpdateAdmin creates or updates an administrator with role super_admin.
func createOrUpdateAdmin(ctx context.Context, db *database.DB, log *slog.Logger, email, password, nameEn, nameAr string) error {
	if len(password) < 8 {
		return fmt.Errorf("admin password must be at least 8 characters")
	}

	cleanEmail := identity.NormalizeEmail(email)
	if cleanEmail == "" {
		return fmt.Errorf("invalid admin email %q", email)
	}

	hash, err := identity.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	nameJSON, err := i18n.New(nameAr, nameEn).Value()
	if err != nil {
		return fmt.Errorf("encode admin name: %w", err)
	}

	return db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var userID int64
		query := `
			INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone, email_verified_at)
			VALUES ($1, $2, $3::jsonb, 'super_admin', 'active', 'ar', 'Africa/Cairo', now())
			ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
				password_hash     = EXCLUDED.password_hash,
				name              = EXCLUDED.name,
				role              = 'super_admin',
				status            = 'active',
				email_verified_at = COALESCE(identity.users.email_verified_at, now()),
				deleted_at        = NULL,
				updated_at        = now()
			RETURNING id;
		`
		if err := tx.QueryRow(txCtx, query, cleanEmail, hash, nameJSON).Scan(&userID); err != nil {
			return fmt.Errorf("upsert super_admin user %s: %w", cleanEmail, err)
		}

		// Ensure user_security record exists and is unlocked
		_, err = tx.Exec(txCtx, `
			INSERT INTO identity.user_security (user_id, login_attempts)
			VALUES ($1, 0)
			ON CONFLICT (user_id) DO UPDATE SET
				login_attempts = 0,
				locked_until   = NULL;
		`, userID)
		if err != nil {
			return fmt.Errorf("upsert user_security for admin %d: %w", userID, err)
		}

		log.InfoContext(txCtx, "super_admin account is ready",
			"user_id", userID,
			"email", cleanEmail,
			"role", "super_admin",
			"status", "active")
		return nil
	})
}
