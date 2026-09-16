package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements notifications.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new notification PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateLog writes a notification delivery log record.
func (r *Repository) CreateLog(ctx context.Context, l *notifications.NotificationLog) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Idempotency guard for the in-app feed: an identical notification (same
		// user, title and body) written again within a short window is almost
		// always an accidental double-send — a compound notifier that reaches the
		// user both directly and through an organization fan-out, a
		// double-submitted form, or a retried request. Collapse it onto the
		// existing row rather than stacking a duplicate the user has to dismiss
		// twice.
		if l.Channel == notifications.ChannelInApp {
			var exID int64
			var exPublic string
			var exCreated time.Time
			err := tx.QueryRow(txCtx, `
				SELECT id, public_id, created_at
				FROM notifications.logs
				WHERE user_id = $1 AND channel = 'in_app' AND title = $2 AND body = $3
				  AND created_at > now() - interval '30 seconds'
				ORDER BY id DESC
				LIMIT 1;
			`, l.UserID, l.Title, l.Body).Scan(&exID, &exPublic, &exCreated)
			if err == nil {
				l.ID, l.PublicID, l.CreatedAt = exID, exPublic, exCreated
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}

		query := `
			INSERT INTO notifications.logs (
				user_id, organization_id, branch_id, channel, recipient, title, body, status, error_message, sent_at, required_permission
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at;
		`
		// error_message is NOT NULL DEFAULT '' as of migration 033. It used to
		// store an empty message as NULL, which is what the read path then could
		// not scan back into a string.
		return tx.QueryRow(txCtx, query,
			l.UserID, l.OrganizationID, l.BranchID, string(l.Channel), l.Recipient,
			l.Title, l.Body, string(l.Status), l.ErrorMessage, l.SentAt, l.RequiredPermission,
		).Scan(&l.ID, &l.PublicID, &l.CreatedAt)
	})
}

// GetTemplateBySlug retrieves a message template by unique slug.
func (r *Repository) GetTemplateBySlug(ctx context.Context, slug string) (*notifications.Template, error) {
	var t notifications.Template
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, channel, title, body, is_active, created_at, updated_at
			FROM notifications.templates
			WHERE slug = $1 AND is_active = true;
		`
		var chStr string
		err := tx.QueryRow(txCtx, query, slug).Scan(
			&t.ID, &t.PublicID, &t.Slug, &chStr, &t.Title, &t.Body, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("template")
			}
			return err
		}
		t.Channel = notifications.Channel(chStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListUserNotifications retrieves paginated in-app feeds for a user.
func (r *Repository) ListUserNotifications(ctx context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		actor, hasActor := authctx.From(ctx)
		isPrivileged := hasActor && (actor.IsSuperAdmin() || actor.Can("*"))

		var query string
		var rows pgx.Rows
		var err error

		if hasActor && !isPrivileged && actor.UserID == userID {
			query = `
				SELECT id, public_id, user_id, organization_id, branch_id, channel, recipient, title, body,
				       required_permission, status, error_message, is_read, read_at, sent_at, created_at
				FROM notifications.logs
				WHERE user_id = $1
				  AND (required_permission = '' OR required_permission = ANY($2))
				ORDER BY created_at DESC
				LIMIT $3 OFFSET $4;
			`
			rows, err = tx.Query(txCtx, query, userID, actor.Permissions, limit, offset)
		} else {
			query = `
				SELECT id, public_id, user_id, organization_id, branch_id, channel, recipient, title, body,
				       required_permission, status, error_message, is_read, read_at, sent_at, created_at
				FROM notifications.logs
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3;
			`
			rows, err = tx.Query(txCtx, query, userID, limit, offset)
		}
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l notifications.NotificationLog
			var chStr, statusStr string
			var errMsg *string
			if err := rows.Scan(
				&l.ID, &l.PublicID, &l.UserID, &l.OrganizationID, &l.BranchID, &chStr, &l.Recipient,
				&l.Title, &l.Body, &l.RequiredPermission, &statusStr, &errMsg, &l.IsRead, &l.ReadAt, &l.SentAt, &l.CreatedAt,
			); err != nil {
				return err
			}
			l.Channel = notifications.Channel(chStr)
			l.Status = notifications.DeliveryStatus(statusStr)
			if errMsg != nil {
				l.ErrorMessage = *errMsg
			}
			list = append(list, &l)
		}
		return rows.Err()
	})
	return list, err
}

// MarkAsRead flags a notification as read.
func (r *Repository) MarkAsRead(ctx context.Context, id int64, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE notifications.logs SET is_read = true, read_at = now() WHERE id = $1 AND user_id = $2;`
		tag, err := tx.Exec(txCtx, query, id, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("notification")
		}
		return nil
	})
}

// GetUnreadCount returns total unread notifications for a user.
func (r *Repository) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		actor, hasActor := authctx.From(ctx)
		isPrivileged := hasActor && (actor.IsSuperAdmin() || actor.Can("*"))

		if hasActor && !isPrivileged && actor.UserID == userID {
			query := `
				SELECT COUNT(*)
				FROM notifications.logs
				WHERE user_id = $1 AND is_read = false
				  AND (required_permission = '' OR required_permission = ANY($2));
			`
			return tx.QueryRow(txCtx, query, userID, actor.Permissions).Scan(&count)
		}

		query := `SELECT COUNT(*) FROM notifications.logs WHERE user_id = $1 AND is_read = false;`
		return tx.QueryRow(txCtx, query, userID).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("notifications postgres: unread count: %w", err)
	}
	return count, nil
}

// HasNotificationWithTitle checks whether a notification with the given title has been sent to the user since the specified time.
func (r *Repository) HasNotificationWithTitle(ctx context.Context, userID int64, title string, since time.Time) (bool, error) {
	var exists bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT EXISTS (
				SELECT 1
				FROM notifications.logs
				WHERE user_id = $1
				  AND title = $2
				  AND created_at >= $3
			);
		`
		return tx.QueryRow(txCtx, query, userID, title, since).Scan(&exists)
	})
	if err != nil {
		return false, fmt.Errorf("notifications postgres: has notification with title: %w", err)
	}
	return exists, nil
}

