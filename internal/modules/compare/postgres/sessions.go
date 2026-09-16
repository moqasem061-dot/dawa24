package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const sessionColumns = `id, public_id, subscription_user_id, user_id, session_id, COALESCE(device_uuid, ''), is_active, COALESCE(device_name, ''), COALESCE(device_type, ''), COALESCE(platform, ''), COALESCE(platform_version, ''), COALESCE(browser, ''), COALESCE(browser_version, ''), ip_address::text, COALESCE(user_agent, ''), COALESCE(country, ''), COALESCE(city, ''), logged_in_at, last_activity_at, logged_out_at, created_at, updated_at`

func scanUserSession(row pgx.Row) (*compare.UserSession, error) {
	var s compare.UserSession
	var ipStr *string
	if err := row.Scan(
		&s.ID, &s.PublicID, &s.SubscriptionUserID, &s.UserID, &s.SessionID, &s.DeviceUUID,
		&s.IsActive, &s.DeviceName, &s.DeviceType, &s.Platform, &s.PlatformVersion,
		&s.Browser, &s.BrowserVersion, &ipStr, &s.UserAgent, &s.Country, &s.City,
		&s.LoggedInAt, &s.LastActivityAt, &s.LoggedOutAt, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if ipStr != nil {
		s.IPAddress = *ipStr
	}
	return &s, nil
}

func (r *Repository) UpsertUserSession(ctx context.Context, sess *compare.UserSession) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		if sess.LoggedInAt.IsZero() {
			sess.LoggedInAt = now
		}
		sess.LastActivityAt = now

		query := `
			INSERT INTO compare.user_sessions (
				subscription_user_id, user_id, session_id, device_uuid, is_active,
				device_name, device_type, platform, platform_version, browser, browser_version,
				ip_address, user_agent, country, city, logged_in_at, last_activity_at
			) VALUES ($1, $2, $3, $4, true, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::inet, $12, $13, $14, $15, $16)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			sess.SubscriptionUserID, sess.UserID, sess.SessionID, sess.DeviceUUID,
			sess.DeviceName, sess.DeviceType, sess.Platform, sess.PlatformVersion, sess.Browser, sess.BrowserVersion,
			sess.IPAddress, sess.UserAgent, sess.Country, sess.City, sess.LoggedInAt, sess.LastActivityAt,
		).Scan(&sess.ID, &sess.PublicID, &sess.CreatedAt, &sess.UpdatedAt)
	})
}

func (r *Repository) TouchUserSession(ctx context.Context, sessionID string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.user_sessions SET last_activity_at = now(), updated_at = now() WHERE session_id = $1 AND is_active = true AND deleted_at IS NULL;`, sessionID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("user session")
		}
		return nil
	})
}

func (r *Repository) CountActiveUserSessions(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*) FROM compare.user_sessions
			WHERE user_id = $1 AND is_active = true AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, userID).Scan(&count)
	})
	return count, err
}

func (r *Repository) ListActiveUserSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	var list []*compare.UserSession
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + sessionColumns + `
			FROM compare.user_sessions
			WHERE user_id = $1 AND is_active = true AND deleted_at IS NULL
			ORDER BY last_activity_at ASC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			s, err := scanUserSession(rows)
			if err != nil {
				return err
			}
			list = append(list, s)
		}
		return rows.Err()
	})
	return list, err
}

// EvictOldestSessions implements Laravel SessionService parity: logs out the oldest active sessions when over the max device cap.
func (r *Repository) EvictOldestSessions(ctx context.Context, userID int64, keepCount int) error {
	if keepCount <= 0 {
		keepCount = 1
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			WITH active AS (
				SELECT id, ROW_NUMBER() OVER (ORDER BY last_activity_at DESC) AS rn
				FROM compare.user_sessions
				WHERE user_id = $1 AND is_active = true AND deleted_at IS NULL
			)
			UPDATE compare.user_sessions
			SET is_active = false, logged_out_at = now(), updated_at = now()
			WHERE id IN (
				SELECT id FROM active WHERE rn > $2
			);
		`
		_, err := tx.Exec(txCtx, query, userID, keepCount)
		return err
	})
}

func (r *Repository) DeactivateUserSession(ctx context.Context, sessionID string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.user_sessions SET is_active = false, logged_out_at = now(), updated_at = now() WHERE session_id = $1 AND is_active = true;`, sessionID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("user session")
		}
		return nil
	})
}
