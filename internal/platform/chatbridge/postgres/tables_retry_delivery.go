package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RetryDelivery requeues a leased message, or fails it at maxAttempts. A zero
// retryAfter backs off exponentially.
func (t Tables) RetryDelivery(ctx context.Context, id int64, errText string, retryAfter time.Duration, maxAttempts int) error {
	return t.exec(ctx, `
		UPDATE {s}.deliveries
		   SET status = CASE WHEN attempts >= $4 THEN 'failed' ELSE 'queued' END,
		       last_error = $2,
		       lease_until = NULL,
		       next_attempt_at = CASE
		           WHEN $3::float8 > 0 THEN now() + make_interval(secs => $3::float8)
		           ELSE now() + make_interval(mins => LEAST(power(2, attempts), 60)::int)
		       END
		 WHERE id = $1 AND status = 'leased';`, id, errText, retryAfter.Seconds(), maxAttempts)
}

// FailDeliveryAndBlockLink records a permanent refusal — the user blocked the
// sender or no longer exists — and stops sending to that chat.
func (t Tables) FailDeliveryAndBlockLink(ctx context.Context, id int64, errText string) error {
	return t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var linkID int64
		err := tx.QueryRow(ctx, t.SQL(`
			UPDATE {s}.deliveries SET status = 'failed', last_error = $2, lease_until = NULL
			 WHERE id = $1 AND status = 'leased'
			RETURNING link_id;`), id, errText).Scan(&linkID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, t.SQL(`
			UPDATE {s}.links SET status = 'blocked', updated_at = now()
			 WHERE id = $1 AND status = 'active';`), linkID)
		return err
	})
}

// exec runs one statement in its own transaction, qualifying "{s}." tables.
func (t Tables) exec(ctx context.Context, sql string, args ...any) error {
	return t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, t.SQL(sql), args...)
		return err
	})
}
