// Package postgres implements telegram.Repository.
//
// The statements every chat channel shares live in chatbridge/postgres; this
// package adds the ones that name a Telegram account. Every statement names
// the user or link it acts on in its WHERE clause (see chatbridge/postgres).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	chatPostgres "github.com/muhiya/dawa24-store/internal/modules/chatbridge/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements telegram.Repository.
type Repository struct {
	chatPostgres.Tables
	db *database.DB
}

// New constructs the repository.
func New(db *database.DB) *Repository {
	return &Repository{Tables: chatPostgres.NewTables(db, "telegram"), db: db}
}

var _ telegram.Repository = (*Repository)(nil)

const linkColumns = `
	id, public_id::text, user_id, telegram_user_id, chat_id, username, display_name,
	status, active_organization_id, conversation_id, muted_categories,
	confirm_expires_at, confirmed_at`

func scanLink(row pgx.Row) (*telegram.Link, error) {
	var (
		l      telegram.Link
		status string
	)
	err := row.Scan(&l.ID, &l.PublicID, &l.UserID, &l.TelegramUserID, &l.ChatID, &l.Username, &l.DisplayName,
		&status, &l.ActiveOrgID, &l.ConversationID, &l.MutedCategories,
		&l.ConfirmExpiresAt, &l.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Status = telegram.LinkStatus(status)
	return &l, nil
}

// LiveLinkByTelegramUser returns the non-revoked link for a Telegram account.
func (r *Repository) LiveLinkByTelegramUser(ctx context.Context, telegramUserID int64) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM telegram.links
			 WHERE telegram_user_id = $1 AND status IN `+chatPostgres.LiveStatuses+`
			 LIMIT 1;`, telegramUserID))
		return err
	})
	return link, err
}

// CurrentLinkForUser returns what the settings page shows: an unexpired
// pending link awaiting confirmation first, otherwise the confirmed one.
func (r *Repository) CurrentLinkForUser(ctx context.Context, userID int64) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM telegram.links
			 WHERE user_id = $1
			   AND (status IN ('active', 'blocked')
			        OR (status = 'pending' AND confirm_expires_at > now()))
			 ORDER BY (status = 'pending') DESC, created_at DESC
			 LIMIT 1;`, userID))
		return err
	})
	return link, err
}

// CreatePendingLink records that a Telegram account opened a user's code.
func (r *Repository) CreatePendingLink(ctx context.Context, l *telegram.Link, confirmBy time.Time) error {
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var (
			existingID     int64
			existingUser   int64
			existingStatus string
		)
		err := tx.QueryRow(ctx, `
			SELECT id, user_id, status FROM telegram.links
			 WHERE telegram_user_id = $1 AND status IN `+chatPostgres.LiveStatuses+`
			 FOR UPDATE;`, l.TelegramUserID).Scan(&existingID, &existingUser, &existingStatus)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case existingStatus != string(telegram.LinkPending) && existingUser != l.UserID:
			return telegram.ErrLinkedElsewhere
		case existingStatus != string(telegram.LinkPending):
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
			INSERT INTO telegram.links
			       (user_id, telegram_user_id, chat_id, username, display_name, status,
			        active_organization_id, confirm_expires_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
			RETURNING id, public_id::text;`,
			l.UserID, l.TelegramUserID, l.ChatID, l.Username, l.DisplayName, l.ActiveOrgID, confirmBy,
		).Scan(&l.ID, &l.PublicID)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return telegram.ErrLinkedElsewhere
	}
	return err
}

// ConfirmPendingLink activates the user's own unexpired pending link and
// retires any Telegram account they had confirmed before.
func (r *Repository) ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `
			SELECT id FROM telegram.links
			 WHERE user_id = $1 AND public_id::text = $2
			   AND status = 'pending' AND confirm_expires_at > now()
			 FOR UPDATE;`, userID, publicID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return telegram.ErrNoPendingLink
		}
		if err != nil {
			return err
		}
		if err := r.RetireOtherLinks(ctx, tx, userID, id); err != nil {
			return err
		}
		link, err = scanLink(tx.QueryRow(ctx, `
			UPDATE telegram.links
			   SET status = 'active', confirmed_at = now(), confirm_expires_at = NULL, updated_at = now()
			 WHERE id = $1
			RETURNING `+linkColumns+`;`, id))
		return err
	})
	return link, err
}

// SetLinkStatus moves a confirmed link between active and blocked.
func (r *Repository) SetLinkStatus(ctx context.Context, linkID int64, status telegram.LinkStatus) error {
	return r.Tables.SetLinkStatus(ctx, linkID, string(status))
}

// TouchLink refreshes what Telegram says about the account.
func (r *Repository) TouchLink(ctx context.Context, linkID, chatID int64, username, displayName string) error {
	return r.exec(ctx, `
		UPDATE telegram.links
		   SET chat_id = $2, username = $3, display_name = $4, last_seen_at = now(), updated_at = now()
		 WHERE id = $1;`, linkID, chatID, username, displayName)
}

// MarkUpdateProcessed reports whether this is the first time an update id has
// been seen.
func (r *Repository) MarkUpdateProcessed(ctx context.Context, updateID int64) (bool, error) {
	var first bool
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO telegram.processed_updates (update_id) VALUES ($1) ON CONFLICT DO NOTHING;`, updateID)
		first = err == nil && tag.RowsAffected() == 1
		return err
	})
	return first, err
}

// PurgeProcessedUpdates forgets update ids older than a time.
func (r *Repository) PurgeProcessedUpdates(ctx context.Context, olderThan time.Time) error {
	return r.exec(ctx, `DELETE FROM telegram.processed_updates WHERE received_at < $1;`, olderThan)
}

func (r *Repository) exec(ctx context.Context, sql string, args ...any) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}
