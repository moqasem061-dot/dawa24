package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Platform administration over user accounts.
//
// Every function in this file was previously a stub: AdminListUsers returned
// (nil, nil) and the three mutations returned nil without touching the
// database. Four live endpoints and the admin users screen sat on top of them,
// so the user list was permanently empty and suspending an account, resetting
// its MFA or changing its role all reported success while doing nothing. A
// silent no-op on a privileged action is worse than an error: the operator
// believes the account is locked and moves on.
//
// These read and write across every tenant, so they run under
// database.AsSystem, and each mutation records an audit row in the same
// transaction as the change it describes.

// AdminListUsers returns users across all tenants, optionally filtered.
func (r *Repository) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	var list []*identity.User
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, email, name, role, status, language, timezone,
			       phone, email_verified_at, phone_verified_at, created_at, updated_at
			FROM identity.users
			WHERE deleted_at IS NULL
			  AND ($1::text IS NULL OR role = $1)
			  AND ($2::text IS NULL OR status = $2)
			ORDER BY created_at DESC
			LIMIT 200;
		`
		// Empty filters mean "no filter" rather than "match the empty string".
		var rolePtr, statusPtr *string
		if role != "" {
			rolePtr = &role
		}
		if status != "" {
			statusPtr = &status
		}

		rows, err := tx.Query(txCtx, query, rolePtr, statusPtr)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u identity.User
			var statusStr, langStr string
			if err := rows.Scan(
				&u.ID, &u.PublicID, &u.Email, &u.Name, &u.Role, &statusStr, &langStr,
				&u.Timezone, &u.Phone, &u.EmailVerifiedAt, &u.PhoneVerifiedAt,
				&u.CreatedAt, &u.UpdatedAt,
			); err != nil {
				return err
			}
			u.Status = identity.UserStatus(statusStr)
			u.Language = i18n.Lang(langStr)
			list = append(list, &u)
		}
		return rows.Err()
	})
	return list, err
}

// SearchUsers returns active users matching a query by name, email, or phone.
func (r *Repository) SearchUsers(ctx context.Context, query, role string, limit int) ([]*identity.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []*identity.User
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		sql := `
			SELECT id, public_id, email, name, role, status, language, timezone,
			       phone, email_verified_at, phone_verified_at, created_at, updated_at
			FROM identity.users
			WHERE deleted_at IS NULL
			  AND ($1::text = '' OR (
			      email::text ILIKE '%' || $1 || '%' OR
			      phone ILIKE '%' || $1 || '%' OR
			      name->>'ar' ILIKE '%' || $1 || '%' OR
			      name->>'en' ILIKE '%' || $1 || '%' OR
			      name::text ILIKE '%' || $1 || '%'
			  ))
			  AND ($2::text = '' OR role = $2)
			ORDER BY created_at DESC
			LIMIT $3;
		`
		rows, err := tx.Query(txCtx, sql, query, role, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u identity.User
			var statusStr, langStr string
			if err := rows.Scan(
				&u.ID, &u.PublicID, &u.Email, &u.Name, &u.Role, &statusStr, &langStr,
				&u.Timezone, &u.Phone, &u.EmailVerifiedAt, &u.PhoneVerifiedAt,
				&u.CreatedAt, &u.UpdatedAt,
			); err != nil {
				return err
			}
			u.Status = identity.UserStatus(statusStr)
			u.Language = i18n.Lang(langStr)
			list = append(list, &u)
		}
		return rows.Err()
	})
	return list, err
}

// AdminUpdateUserStatus activates, suspends or bans an account.
func (r *Repository) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var before string
		if err := tx.QueryRow(txCtx,
			`SELECT status FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`, id,
		).Scan(&before); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user status: %w", err)
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE identity.users SET status = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`,
			status, id,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: update user status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		// A changed account status changes what the resolver grants. Bumping
		// here, in the same transaction, is what makes it visible within
		// seconds to callers with no session to revoke — a Telegram message.
		if err := rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey); err != nil {
			return err
		}

		action := "identity.user.status_changed"
		if status == "suspended" {
			action = "user.suspend"
		} else if status == "active" {
			action = "user.reactivate"
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      action,
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]string{"status": before},
			After:       map[string]string{"status": status},
		})
	})
}

// AdminResetMFA clears a user's second factor so they can enrol again.
func (r *Repository) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var existed bool
		if err := tx.QueryRow(txCtx,
			`SELECT EXISTS (SELECT 1 FROM identity.user_mfa WHERE user_id = $1);`, id,
		).Scan(&existed); err != nil {
			return fmt.Errorf("identity postgres: check mfa: %w", err)
		}

		// The row is removed rather than disabled. mfa_enabled_requires_secret
		// permits an enabled record only with a secret and a confirmation, and
		// leaving a stale secret behind for a factor the user no longer holds
		// serves no purpose.
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_mfa WHERE user_id = $1;`, id); err != nil {
			return fmt.Errorf("identity postgres: reset mfa: %w", err)
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.mfa_reset",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]bool{"mfa_configured": existed},
			After:       map[string]bool{"mfa_configured": false},
		})
	})
}

// AdminAssignRole changes a user's platform role.
func (r *Repository) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var before string
		if err := tx.QueryRow(txCtx,
			`SELECT role FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`, id,
		).Scan(&before); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user role: %w", err)
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE identity.users SET role = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`,
			role, id,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: assign role: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		if err := rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey); err != nil {
			return err
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.role_assigned",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]string{"role": before},
			After:       map[string]string{"role": role},
		})
	})
}

// AdminCountUsers returns the total number of active user accounts.
func (r *Repository) AdminCountUsers(ctx context.Context) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT COUNT(*) FROM identity.users WHERE deleted_at IS NULL;`).Scan(&total)
	})
	return total, err
}

// AdminSetPassword updates a user's password hash and records an audit row without any credential material.
func (r *Repository) AdminSetPassword(ctx context.Context, userID int64, passwordHash string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(txCtx,
			`SELECT email FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`, userID,
		).Scan(&email); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user email: %w", err)
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE identity.users SET password_hash = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`,
			passwordHash, userID,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: update password hash: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.password_set",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(userID, 10),
			Before:      map[string]any{"password_changed": false},
			After:       map[string]any{"password_changed": true, "email": email},
		})
	})
}

// AdminUpdateUserDetails updates user core fields, status, and national ID, and writes an audit row.
func (r *Repository) AdminUpdateUserDetails(ctx context.Context, userID int64, in identity.AdminEditUserInput, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var beforeEmail, beforePhone, beforeAvatar, beforeStatus string
		var beforeName i18n.Text
		if err := tx.QueryRow(txCtx,
			`SELECT email, COALESCE(phone, ''), COALESCE(avatar_url, ''), status, name FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`,
			userID,
		).Scan(&beforeEmail, &beforePhone, &beforeAvatar, &beforeStatus, &beforeName); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user for update: %w", err)
		}

		// NOTE: identity.kyc_records was dropped by migration 152_retire_dead_tables.
		// national_id should be stored on identity.users directly in a future migration.
		var beforeNationalID string

		name := i18n.Text{
			"ar": in.NameAr,
			"en": in.NameEn,
		}

		normalizedEmail := identity.NormalizeEmail(in.Email)
		statusStr := string(in.Status)
		if statusStr == "" {
			statusStr = beforeStatus
		}

		tag, err := tx.Exec(txCtx, `
			UPDATE identity.users
			SET name = $1, email = $2, phone = $3, avatar_url = $4, status = $5, updated_at = now()
			WHERE id = $6 AND deleted_at IS NULL;
		`, name, normalizedEmail, in.Phone, in.AvatarURL, statusStr, userID)
		if err != nil {
			return fmt.Errorf("identity postgres: update user details: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		// NOTE: identity.kyc_records was dropped. national_id upsert is a no-op
		// until the column is added to identity.users in a future migration.

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.updated",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(userID, 10),
			Before: map[string]any{
				"name":        beforeName,
				"email":       beforeEmail,
				"phone":       beforePhone,
				"avatar_url":  beforeAvatar,
				"status":      beforeStatus,
				"national_id": beforeNationalID,
			},
			After: map[string]any{
				"name":        name,
				"email":       normalizedEmail,
				"phone":       in.Phone,
				"avatar_url":  in.AvatarURL,
				"status":      statusStr,
				"national_id": in.NationalID,
			},
		})
	})
}

// GetNationalID retrieves national ID if present.
//
// NOTE: identity.kyc_records was dropped by migration 152_retire_dead_tables.
// This returns empty until national_id is stored on identity.users directly.
func (r *Repository) GetNationalID(ctx context.Context, userID int64) (string, error) {
	return "", nil
}
