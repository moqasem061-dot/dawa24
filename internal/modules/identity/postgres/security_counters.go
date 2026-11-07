package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// RecordMFAFailure counts one wrong second-factor code in a single statement,
// so concurrent guesses cannot overwrite each other's count. A lock that has
// already expired starts the count again at one.
func (r *Repository) RecordMFAFailure(ctx context.Context, userID int64, now time.Time, threshold int, lockFor time.Duration) (int, *time.Time, error) {
	const next = `CASE WHEN identity.user_security.mfa_locked_until IS NOT NULL
		AND identity.user_security.mfa_locked_until <= $2::timestamptz
		THEN 1 ELSE identity.user_security.mfa_failed_attempts + 1 END`
	query := `
		INSERT INTO identity.user_security (user_id, mfa_failed_attempts, mfa_locked_until)
		VALUES ($1, 1, CASE WHEN 1 >= $3::int THEN $4::timestamptz END)
		ON CONFLICT (user_id) DO UPDATE SET
			mfa_failed_attempts = ` + next + `,
			mfa_locked_until = CASE WHEN (` + next + `) >= $3::int THEN $4::timestamptz END
		RETURNING mfa_failed_attempts, mfa_locked_until;`

	var attempts int
	var lockedUntil *time.Time
	lockAt := now.Add(lockFor)
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, query, userID, now, threshold, lockAt).Scan(&attempts, &lockedUntil)
	})
	if err != nil {
		return 0, nil, fmt.Errorf("identity postgres: record mfa failure: %w", err)
	}
	return attempts, lockedUntil, nil
}

// ClearMFAFailures resets the second-factor counter and any second-factor lock.
func (r *Repository) ClearMFAFailures(ctx context.Context, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE identity.user_security
			SET mfa_failed_attempts = 0, mfa_locked_until = NULL
			WHERE user_id = $1;`, userID)
		if err != nil {
			return fmt.Errorf("identity postgres: clear mfa failures: %w", err)
		}
		return nil
	})
}

// StampPasswordChange writes last_password_change only when it is unset or
// older than tokenIssuedAt. The conditional upsert is one statement, so two
// submissions of the same reset token cannot both succeed.
func (r *Repository) StampPasswordChange(ctx context.Context, userID int64, changedAt, tokenIssuedAt time.Time) (bool, error) {
	var written bool
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			INSERT INTO identity.user_security (user_id, last_password_change)
			VALUES ($1, $2::timestamptz)
			ON CONFLICT (user_id) DO UPDATE SET last_password_change = EXCLUDED.last_password_change
			WHERE identity.user_security.last_password_change IS NULL
				OR identity.user_security.last_password_change < $3::timestamptz;`,
			userID, changedAt, tokenIssuedAt)
		if err != nil {
			return fmt.Errorf("identity postgres: stamp password change: %w", err)
		}
		written = tag.RowsAffected() > 0
		return nil
	})
	return written, err
}
