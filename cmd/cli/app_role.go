package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// appRole is the least-privilege role the server and worker connect as.
const appRole = "dawa24_app"

// migrationDefaultAppPassword is the password migration 222 created appRole
// with. It is in the repository, so a role still accepting it is open to anyone
// who can reach the database.
const migrationDefaultAppPassword = "dawa24_app_change_me"

// setAppRolePassword keeps appRole's password out of the repository.
//
// With DB_APP_PASSWORD set, migrate (which runs as the schema owner) sets it,
// so rotating the password is a config change plus a deploy. Without it, the
// role is probed with the migration's default; still accepting that password
// fails a production migrate, because the next step is somebody pointing
// APP_DATABASE_URL at it.
func setAppRolePassword(ctx context.Context, db *database.DB, log *slog.Logger, prod bool) error {
	var exists bool
	if err := db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, appRole).Scan(&exists)
	}); err != nil {
		return fmt.Errorf("look up %s: %w", appRole, err)
	}
	if !exists {
		return nil
	}

	if pw := os.Getenv("DB_APP_PASSWORD"); pw != "" {
		stmt := fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD %s", pgx.Identifier{appRole}.Sanitize(), database.QuoteLiteral(pw))
		if err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx, stmt)
			return err
		}); err != nil {
			return fmt.Errorf("set %s password: %w", appRole, err)
		}
		log.InfoContext(ctx, "application role password set from DB_APP_PASSWORD", "role", appRole)
		return nil
	}

	if !acceptsPassword(ctx, os.Getenv("DATABASE_URL"), appRole, migrationDefaultAppPassword) {
		return nil
	}
	msg := appRole + " still accepts the default password from migration 222; set DB_APP_PASSWORD and re-run migrate"
	if prod {
		return errors.New(msg)
	}
	log.WarnContext(ctx, msg)
	return nil
}

// acceptsPassword reports whether role can sign in with password on the
// database dsn points at.
func acceptsPassword(ctx context.Context, dsn, role, password string) bool {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return false
	}
	u.User = url.UserPassword(role, password)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		return false
	}
	_ = conn.Close(ctx)
	return true
}
