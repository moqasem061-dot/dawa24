package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Repository implements identity.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL identity repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateUser inserts a new user record.
func (r *Repository) CreateUser(ctx context.Context, u *identity.User) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO identity.users (
				email, password_hash, name, role, status, language, timezone, phone, phone_verified_at
			) VALUES (
				-- name is NOT NULL DEFAULT '{"ar":"","en":""}', so the schema already
				-- treats an empty name as acceptable. An empty i18n.Text marshals to
				-- NULL, though, which violates the constraint instead of taking the
				-- default. Registration validates the name; this keeps any other
				-- caller from turning a missing one into a 500.
				$1, $2, COALESCE($3, '{"ar":"","en":""}'::jsonb), $4, $5, $6, $7, $8, $9
			) RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			identity.NormalizeEmail(u.Email),
			u.PasswordHash,
			u.Name,
			u.Role,
			string(u.Status),
			string(u.Language),
			u.Timezone,
			u.Phone,
			u.PhoneVerifiedAt,
		).Scan(&u.ID, &u.PublicID, &u.CreatedAt, &u.UpdatedAt)

		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("user.email_exists", "An account with this email already exists.")
			}
			return fmt.Errorf("identity postgres: create user: %w", err)
		}

		var isStaff bool
		_ = tx.QueryRow(txCtx, `SELECT is_staff FROM identity.roles WHERE key = $1 AND deleted_at IS NULL;`, u.Role).Scan(&isStaff)
		if isStaff {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				Action:     "user.create_staff",
				EntityType: "identity.user",
				EntityID:   strconv.FormatInt(u.ID, 10),
				After:      map[string]any{"email": u.Email, "role": u.Role},
			})
		}

		return nil
	})
}

// GetUserByID retrieves an active user by primary key.
// GetUserByID reads one user.
//
// Outside a transaction: identity.users carries no row-level security policy
// (see database.UnscopedTables and the test that checks that against the live
// schema), so the BEGIN, the set_config and the COMMIT that used to wrap this
// were three network round trips guarding nothing. It matters here more than
// almost anywhere else because session validation calls this on EVERY
// authenticated request — pg_stat_user_tables recorded 42,300 scans of a
// seventeen-row table — so the saving is three round trips per request.
func (r *Repository) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	const query = `
		SELECT id, public_id, email, password_hash, name, role, status, language, timezone,
		       phone, COALESCE(avatar_url, ''), email_verified_at, phone_verified_at, created_at, updated_at, deleted_at
		FROM identity.users
		WHERE id = $1 AND deleted_at IS NULL;
	`
	var (
		u                  identity.User
		statusStr, langStr string
	)
	err := r.db.QueryRowUnscoped(ctx, query, id).Scan(
		&u.ID, &u.PublicID, &u.Email, &u.PasswordHash, &u.Name, &u.Role,
		&statusStr, &langStr, &u.Timezone, &u.Phone, &u.AvatarURL, &u.EmailVerifiedAt,
		&u.PhoneVerifiedAt, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("user")
		}
		return nil, fmt.Errorf("identity postgres: get user by id: %w", err)
	}
	u.Status = identity.UserStatus(statusStr)
	u.Language = i18n.Lang(langStr)
	return &u, nil
}

// GetUserByEmail retrieves a user by email address.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	var u identity.User
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, email, password_hash, name, role, status, language, timezone,
			       phone, COALESCE(avatar_url, ''), email_verified_at, phone_verified_at, created_at, updated_at, deleted_at
			FROM identity.users
			WHERE email = $1 AND deleted_at IS NULL;
		`
		var statusStr, langStr string
		err := tx.QueryRow(txCtx, query, identity.NormalizeEmail(email)).Scan(
			&u.ID, &u.PublicID, &u.Email, &u.PasswordHash, &u.Name, &u.Role,
			&statusStr, &langStr, &u.Timezone, &u.Phone, &u.AvatarURL, &u.EmailVerifiedAt,
			&u.PhoneVerifiedAt, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: get user by email: %w", err)
		}
		u.Status = identity.UserStatus(statusStr)
		u.Language = i18n.Lang(langStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser updates user profile and security fields.
func (r *Repository) UpdateUser(ctx context.Context, u *identity.User) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE identity.users
			SET email = $2, password_hash = $3,
			    -- Same NOT NULL guard as CreateUser: an empty i18n.Text marshals to
			    -- NULL, which the column rejects rather than defaulting.
			    name = COALESCE($4, '{"ar":"","en":""}'::jsonb), role = $5, status = $6,
			    language = $7, timezone = $8, phone = $9, avatar_url = $10, email_verified_at = $11,
			    phone_verified_at = $12, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var oldHash string
		_ = tx.QueryRow(txCtx, `SELECT password_hash FROM identity.users WHERE id = $1;`, u.ID).Scan(&oldHash)

		res, err := tx.Exec(txCtx, query,
			u.ID, identity.NormalizeEmail(u.Email), u.PasswordHash, u.Name, u.Role,
			string(u.Status), string(u.Language), u.Timezone, u.Phone, u.AvatarURL,
			u.EmailVerifiedAt, u.PhoneVerifiedAt,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: update user: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		if oldHash != "" && u.PasswordHash != oldHash {
			_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
				Action:     "user.password_set",
				EntityType: "identity.user",
				EntityID:   strconv.FormatInt(u.ID, 10),
				After:      map[string]any{"email": u.Email},
			})
		}
		return nil
	})
}

// GetSecurity retrieves login security status for a user.
func (r *Repository) GetSecurity(ctx context.Context, userID int64) (*identity.UserSecurity, error) {
	var s identity.UserSecurity
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT user_id, login_attempts, locked_until, last_login_at, last_login_ip::text,
			       last_user_agent, last_password_change, password_change_count, max_login_sessions
			FROM identity.user_security
			WHERE user_id = $1;
		`
		var ip *string
		err := tx.QueryRow(txCtx, query, userID).Scan(
			&s.UserID, &s.LoginAttempts, &s.LockedUntil, &s.LastLoginAt, &ip,
			&s.LastUserAgent, &s.LastPasswordChange, &s.PasswordChangeCount, &s.MaxLoginSessions,
		)
		if err != nil {
			if database.IsNotFound(err) {
				s.UserID = userID
				return nil
			}
			return fmt.Errorf("identity postgres: get user security: %w", err)
		}
		if ip != nil {
			s.LastLoginIP = *ip
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertSecurity updates login attempts and lockouts.
func (r *Repository) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO identity.user_security (
				user_id, login_attempts, locked_until, last_login_at, last_user_agent,
				last_password_change, password_change_count, max_login_sessions
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8
			) ON CONFLICT (user_id) DO UPDATE SET
				login_attempts = EXCLUDED.login_attempts,
				locked_until = EXCLUDED.locked_until,
				last_login_at = EXCLUDED.last_login_at,
				last_user_agent = EXCLUDED.last_user_agent,
				last_password_change = EXCLUDED.last_password_change,
				password_change_count = EXCLUDED.password_change_count,
				max_login_sessions = EXCLUDED.max_login_sessions;
		`
		_, err := tx.Exec(txCtx, query,
			s.UserID, s.LoginAttempts, s.LockedUntil, s.LastLoginAt, s.LastUserAgent,
			s.LastPasswordChange, s.PasswordChangeCount, s.MaxLoginSessions,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: upsert security: %w", err)
		}
		return nil
	})
}

// GetMFA retrieves MFA configuration.
func (r *Repository) GetMFA(ctx context.Context, userID int64) (*identity.UserMFA, error) {
	var m identity.UserMFA
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT user_id, totp_secret, recovery_codes, enabled, confirmed_at
			FROM identity.user_mfa
			WHERE user_id = $1;
		`
		err := tx.QueryRow(txCtx, query, userID).Scan(
			&m.UserID, &m.TOTPSecret, &m.RecoveryCodes, &m.Enabled, &m.ConfirmedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				m.UserID = userID
				return nil
			}
			return fmt.Errorf("identity postgres: get mfa: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpsertMFA creates or updates MFA records.
func (r *Repository) UpsertMFA(ctx context.Context, m *identity.UserMFA) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO identity.user_mfa (
				user_id, totp_secret, recovery_codes, enabled, confirmed_at
			) VALUES (
				$1, $2, $3, $4, $5
			) ON CONFLICT (user_id) DO UPDATE SET
				totp_secret = EXCLUDED.totp_secret,
				recovery_codes = EXCLUDED.recovery_codes,
				enabled = EXCLUDED.enabled,
				confirmed_at = EXCLUDED.confirmed_at;
		`
		_, err := tx.Exec(txCtx, query, m.UserID, m.TOTPSecret, m.RecoveryCodes, m.Enabled, m.ConfirmedAt)
		if err != nil {
			return fmt.Errorf("identity postgres: upsert mfa: %w", err)
		}
		return nil
	})
}

// GetPermissionsForUser resolves effective permissions across platform roles and active org roles.
func (r *Repository) GetPermissionsForUser(ctx context.Context, userID int64, orgID int64) ([]string, error) {
	var permissions []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT DISTINCT p.key
			FROM identity.permissions p
			JOIN identity.role_permissions rp ON rp.permission_key = p.key
			WHERE rp.role_key IN (
				SELECT role FROM identity.users WHERE id = $1
				UNION
				SELECT role_key FROM org.members WHERE user_id = $1 AND organization_id = $2 AND status = 'active'
			);
		`
		rows, err := tx.Query(txCtx, query, userID, orgID)
		if err != nil {
			return fmt.Errorf("identity postgres: get permissions: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var perm string
			if err := rows.Scan(&perm); err != nil {
				return err
			}
			permissions = append(permissions, perm)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

// GetRolesForUser returns all assigned platform role keys.
func (r *Repository) GetRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	var roles []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT role FROM identity.users WHERE id = $1;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return fmt.Errorf("identity postgres: get roles: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var role string
			if err := rows.Scan(&role); err != nil {
				return err
			}
			roles = append(roles, role)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// DefaultOrgForUser returns the organization to make active at sign-in.
//
// Login only set an active organization when the caller named one, and the web
// sign-in form has no field for it, so every user arrived with organization 0.
// The vendor screens filter on the actor's organization, so a supplier logged
// in through the UI saw an empty catalogue of their own products.
//
// A user in exactly one organization has no choice to make, so it is chosen for
// them. A user in several gets their oldest membership as the opening context
// and can switch with X-Dawa-Org-ID, which is verified against membership.
// Returns 0 when the user belongs to none, which is correct for a customer
// buying as an individual.
func (r *Repository) DefaultOrgForUser(ctx context.Context, userID int64) (int64, error) {
	orgID, _, _, err := r.DefaultOrgInfoForUser(ctx, userID)
	return orgID, err
}
