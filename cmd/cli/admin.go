package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

const defaultAdminEmail = "admin@dawa24.net"

// ensureAdminAccount creates the ADMIN_EMAIL super_admin when it does not exist
// yet, and leaves an existing one untouched.
//
// It runs on every "migrate", which is every deploy. It used to upsert, which
// reset the admin's password to ADMIN_PASSWORD (or to a default written in this
// file) on each release: a password changed through the UI silently reverted,
// and a production deploy without ADMIN_PASSWORD got a publicly known one.
// Resetting a password is `cli create-admin`'s job, and it is explicit.
func ensureAdminAccount(ctx context.Context, db *database.DB, log *slog.Logger, prod bool) error {
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = defaultAdminEmail
	}

	var exists bool
	err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT EXISTS (SELECT 1 FROM identity.users WHERE email = $1 AND deleted_at IS NULL)`,
			identity.NormalizeEmail(email)).Scan(&exists)
	})
	if err != nil {
		return fmt.Errorf("look up admin account: %w", err)
	}
	if exists {
		log.InfoContext(ctx, "super_admin account already exists; leaving it untouched", "email", email)
		return nil
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		// No default password in any environment: a password written in this
		// repository is a password everyone has.
		log.WarnContext(ctx, "no super_admin account and ADMIN_PASSWORD is not set; "+
			"set ADMIN_PASSWORD and run: cli create-admin <email>", "email", email, "prod", prod)
		return nil
	}
	return createOrUpdateAdmin(ctx, db, log, email, password, "Platform Super Admin", "مدير النظام العام")
}

// createOrUpdateAdmin creates or updates an administrator with role super_admin.
func createOrUpdateAdmin(ctx context.Context, db *database.DB, log *slog.Logger, email, password, nameEn, nameAr string) error {
	// The platform owner: stronger than the ordinary account minimum.
	if len(password) < 14 {
		return fmt.Errorf("admin password must be at least 14 characters")
	}
	if err := identity.ValidatePassword(password); err != nil {
		return fmt.Errorf("admin password: %w", err)
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
