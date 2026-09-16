// Package postgres implements whatsapp.Repository.
//
// The statements every chat channel shares live in chatbridge/postgres; this
// package adds the ones that name a WhatsApp number or its 24-hour window.
// Every statement names the user or link it acts on in its WHERE clause (see
// chatbridge/postgres).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	chatPostgres "github.com/muhiya/dawa24-store/internal/modules/chatbridge/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements whatsapp.Repository.
type Repository struct {
	chatPostgres.Tables
	db *database.DB
}

// New constructs the repository.
func New(db *database.DB) *Repository {
	return &Repository{Tables: chatPostgres.NewTables(db, "whatsapp"), db: db}
}

var _ whatsapp.Repository = (*Repository)(nil)

// windowOpenSQL is the 24-hour customer-service window, less a margin for the
// time between WhatsApp receiving a message and this server recording it.
const windowOpenSQL = `COALESCE(k.last_inbound_at > now() - interval '23 hours 30 minutes', false)`

const linkColumns = `
	id, public_id::text, user_id, wa_id, display_name,
	status, active_organization_id, conversation_id, muted_categories,
	confirm_expires_at, confirmed_at`

func scanLink(row pgx.Row) (*whatsapp.Link, error) {
	var (
		l      whatsapp.Link
		status string
	)
	err := row.Scan(&l.ID, &l.PublicID, &l.UserID, &l.WAID, &l.DisplayName,
		&status, &l.ActiveOrgID, &l.ConversationID, &l.MutedCategories,
		&l.ConfirmExpiresAt, &l.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Status = whatsapp.LinkStatus(status)
	return &l, nil
}

// LiveLinkByWAID returns the non-revoked link for a WhatsApp number.
func (r *Repository) LiveLinkByWAID(ctx context.Context, waID string) (*whatsapp.Link, error) {
	var link *whatsapp.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM whatsapp.links
			 WHERE wa_id = $1 AND status IN `+chatPostgres.LiveStatuses+`
			 LIMIT 1;`, waID))
		return err
	})
	return link, err
}

// CurrentLinkForUser returns what the settings page shows: an unexpired
// pending link awaiting confirmation first, otherwise the confirmed one.
func (r *Repository) CurrentLinkForUser(ctx context.Context, userID int64) (*whatsapp.Link, error) {
	var link *whatsapp.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM whatsapp.links
			 WHERE user_id = $1
			   AND (status IN ('active', 'blocked')
			        OR (status = 'pending' AND confirm_expires_at > now()))
			 ORDER BY (status = 'pending') DESC, created_at DESC
			 LIMIT 1;`, userID))
		return err
	})
	return link, err
}

// CreatePendingLink records that a WhatsApp number sent a user's code. The
// message that carried the code opens the 24-hour window.
func (r *Repository) CreatePendingLink(ctx context.Context, l *whatsapp.Link, confirmBy time.Time) error {
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var (
			existingID     int64
			existingUser   int64
			existingStatus string
		)
		err := tx.QueryRow(ctx, `
			SELECT id, user_id, status FROM whatsapp.links
			 WHERE wa_id = $1 AND status IN `+chatPostgres.LiveStatuses+`
			 FOR UPDATE;`, l.WAID).Scan(&existingID, &existingUser, &existingStatus)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case existingStatus != string(whatsapp.LinkPending) && existingUser != l.UserID:
			return whatsapp.ErrLinkedElsewhere
		case existingStatus != string(whatsapp.LinkPending):
			l.ID = existingID
			return nil
		default:
			if err := r.RevokePendingLink(ctx, tx, existingID); err != nil {
				return err
			}
		}

		if err := r.RevokePendingForUser(ctx, tx, l.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO whatsapp.links
			       (user_id, wa_id, display_name, status, active_organization_id,
			        confirm_expires_at, last_inbound_at)
			VALUES ($1, $2, $3, 'pending', $4, $5, now())
			RETURNING id, public_id::text;`,
			l.UserID, l.WAID, l.DisplayName, l.ActiveOrgID, confirmBy,
		).Scan(&l.ID, &l.PublicID)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return whatsapp.ErrLinkedElsewhere
	}
	return err
}

// ConfirmPendingLink activates the user's own unexpired pending link and
// retires any WhatsApp number they had confirmed before.
func (r *Repository) ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*whatsapp.Link, error) {
	var link *whatsapp.Link
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `
			SELECT id FROM whatsapp.links
			 WHERE user_id = $1 AND public_id::text = $2
			   AND status = 'pending' AND confirm_expires_at > now()
			 FOR UPDATE;`, userID, publicID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return whatsapp.ErrNoPendingLink
		}
		if err != nil {
			return err
		}
		if err := r.RetireOtherLinks(ctx, tx, userID, id); err != nil {
			return err
		}
		link, err = scanLink(tx.QueryRow(ctx, `
			UPDATE whatsapp.links
			   SET status = 'active', confirmed_at = now(), confirm_expires_at = NULL, updated_at = now()
			 WHERE id = $1
			RETURNING `+linkColumns+`;`, id))
		return err
	})
	return link, err
}

// SetLinkStatus moves a confirmed link between active and blocked.
func (r *Repository) SetLinkStatus(ctx context.Context, linkID int64, status whatsapp.LinkStatus) error {
	return r.Tables.SetLinkStatus(ctx, linkID, string(status))
}

// TouchLink records an inbound message from the number.
func (r *Repository) TouchLink(ctx context.Context, linkID int64, displayName string) error {
	return r.exec(ctx, `
		UPDATE whatsapp.links
		   SET display_name = COALESCE(NULLIF($2, ''), display_name),
		       last_inbound_at = now(), last_seen_at = now(), updated_at = now()
		 WHERE id = $1;`, linkID, displayName)
}

// MarkMessageProcessed reports whether this is the first time a message id
// has been seen.
func (r *Repository) MarkMessageProcessed(ctx context.Context, messageID string) (bool, error) {
	var first bool
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO whatsapp.processed_messages (message_id) VALUES ($1) ON CONFLICT DO NOTHING;`, messageID)
		first = err == nil && tag.RowsAffected() == 1
		return err
	})
	return first, err
}

// PurgeProcessedMessages forgets message ids older than a time.
func (r *Repository) PurgeProcessedMessages(ctx context.Context, olderThan time.Time) error {
	return r.exec(ctx, `DELETE FROM whatsapp.processed_messages WHERE received_at < $1;`, olderThan)
}

// ClaimDeliveries leases due messages for active links, with whether each
// link's 24-hour window is open.
func (r *Repository) ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]whatsapp.LeasedDelivery, error) {
	var out []whatsapp.LeasedDelivery
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := r.ExpireLeases(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, r.DueDeliveriesSQL()+`d.id, k.wa_id, d.text, `+windowOpenSQL+`;`,
			limit, lease.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d whatsapp.LeasedDelivery
			if err := rows.Scan(&d.ID, &d.WAID, &d.Text, &d.WindowOpen); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// DropDelivery gives up on a leased delivery that can never be sent.
func (r *Repository) DropDelivery(ctx context.Context, id int64, reason string) error {
	return r.exec(ctx, `
		UPDATE whatsapp.deliveries SET status = 'dropped', drop_reason = $2, lease_until = NULL
		 WHERE id = $1 AND status = 'leased';`, id, reason)
}

// CloseWindowAndRetry forgets the link's last inbound message, because
// WhatsApp says the window has closed, and requeues the delivery.
func (r *Repository) CloseWindowAndRetry(ctx context.Context, id int64, errText string, maxAttempts int) error {
	if err := r.exec(ctx, `
		UPDATE whatsapp.links k SET last_inbound_at = NULL, updated_at = now()
		  FROM whatsapp.deliveries d
		 WHERE d.id = $1 AND d.status = 'leased' AND k.id = d.link_id;`, id); err != nil {
		return err
	}
	return r.RetryDelivery(ctx, id, errText, time.Second, maxAttempts)
}

func (r *Repository) exec(ctx context.Context, sql string, args ...any) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}
