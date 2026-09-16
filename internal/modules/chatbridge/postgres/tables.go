// Package postgres is the persistence every chat channel shares.
//
// Each channel has its own schema with the same shape (migrations 213 and
// 219): link_tokens, links and deliveries. What differs — the column naming
// the account on the other side, and what a delivery carries — stays in the
// channel's repository; the statements here are the ones whose meaning must be
// identical everywhere: how a code is spent, how a link is revoked, how a
// delivery is leased, retried and given up on.
//
// Every statement names the user or link it acts on in its WHERE clause.
// These tables are closed to everything but system callers, and the services
// reach them through database.AsSystem, so the predicates here are the whole
// of the scoping — there is no tenant GUC to fall back on.
package postgres

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Tables are one channel's link and delivery tables.
type Tables struct {
	db     *database.DB
	schema string
}

var schemaName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// NewTables binds the shared statements to a channel's schema. The schema is
// spliced into SQL, so it must be a compile-time constant of the channel.
func NewTables(db *database.DB, schema string) Tables {
	if !schemaName.MatchString(schema) {
		panic("chatbridge/postgres: invalid schema name " + schema)
	}
	return Tables{db: db, schema: schema}
}

// DB is the pool the tables live in, for the channel's own statements.
func (t Tables) DB() *database.DB { return t.db }

// SQL qualifies "{s}." table references with the channel's schema.
func (t Tables) SQL(sql string) string { return strings.ReplaceAll(sql, "{s}.", t.schema+".") }

// LiveStatuses are the link statuses that still speak for a user.
const LiveStatuses = `('pending', 'active', 'blocked')`

// staleNotificationSQL is how long a queued notification stays worth sending.
// Kept in step with the channels' notificationWindow.
const staleNotificationSQL = `interval '2 hours'`

// maxLeaseAttempts fails a delivery whose lease has expired this many times —
// n8n claimed it and never reported back, repeatedly.
const maxLeaseAttempts = 5

// orgNameSQL is the organisation's display name, Arabic first.
const orgNameSQL = `COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), o.legal_name, '')`

// CountLinkTokensSince counts codes a user created since a time.
func (t Tables) CountLinkTokensSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	var n int
	err := t.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			t.SQL(`SELECT count(*) FROM {s}.link_tokens WHERE user_id = $1 AND created_at >= $2;`),
			userID, since).Scan(&n)
	})
	return n, err
}

// CreateLinkToken stores a code's hash and retires the user's older codes.
func (t Tables) CreateLinkToken(ctx context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error {
	return t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, t.SQL(`
			UPDATE {s}.link_tokens SET expires_at = now()
			 WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > now();`), userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, t.SQL(`
			INSERT INTO {s}.link_tokens (user_id, organization_id, token_hash, expires_at)
			VALUES ($1, $2, $3, $4);`), userID, orgID, hash, expires)
		return err
	})
}

// ConsumeLinkToken spends a code exactly once. The UPDATE is the check: two
// simultaneous uses race on one row and only one of them gets it back.
func (t Tables) ConsumeLinkToken(ctx context.Context, hash []byte) (int64, *int64, error) {
	var (
		userID int64
		orgID  *int64
	)
	err := t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, t.SQL(`
			UPDATE {s}.link_tokens SET consumed_at = now()
			 WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now()
			RETURNING user_id, organization_id;`), hash).Scan(&userID, &orgID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, chatbridge.ErrTokenInvalid
	}
	return userID, orgID, err
}

// RevokeLink ends one of the user's links and drops anything still queued.
func (t Tables) RevokeLink(ctx context.Context, userID, linkID int64) error {
	return t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, t.SQL(`
			UPDATE {s}.links
			   SET status = 'revoked', revoked_at = now(), busy_until = NULL, updated_at = now()
			 WHERE id = $1 AND user_id = $2 AND status <> 'revoked';`), linkID, userID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(ctx, t.SQL(`
			UPDATE {s}.deliveries SET status = 'dropped', drop_reason = 'link_revoked', lease_until = NULL
			 WHERE link_id = $1 AND status IN ('queued', 'leased');`), linkID)
		return err
	})
}

// RetireOtherLinks revokes a user's other confirmed links inside the
// transaction that confirms a new one.
func (t Tables) RetireOtherLinks(ctx context.Context, tx pgx.Tx, userID, keepID int64) error {
	_, err := tx.Exec(ctx, t.SQL(`
		UPDATE {s}.links SET status = 'revoked', revoked_at = now(), busy_until = NULL, updated_at = now()
		 WHERE user_id = $1 AND status IN ('active', 'blocked') AND id <> $2;`), userID, keepID)
	return err
}

// RevokePendingLink revokes one pending link inside a transaction.
func (t Tables) RevokePendingLink(ctx context.Context, tx pgx.Tx, linkID int64) error {
	_, err := tx.Exec(ctx, t.SQL(`
		UPDATE {s}.links SET status = 'revoked', revoked_at = now(), updated_at = now()
		 WHERE id = $1 AND status = 'pending';`), linkID)
	return err
}

// RevokePendingForUser revokes a user's pending links inside a transaction:
// one pending confirmation per user, the newest code opened wins.
func (t Tables) RevokePendingForUser(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, t.SQL(`
		UPDATE {s}.links SET status = 'revoked', revoked_at = now(), updated_at = now()
		 WHERE user_id = $1 AND status = 'pending';`), userID)
	return err
}

// SetLinkStatus moves a confirmed link between active and blocked. It cannot
// confirm a pending link or revive a revoked one.
func (t Tables) SetLinkStatus(ctx context.Context, linkID int64, status string) error {
	if status != "active" && status != "blocked" {
		return errors.New("chatbridge: SetLinkStatus only toggles active and blocked")
	}
	return t.exec(ctx, `
		UPDATE {s}.links SET status = $2, updated_at = now()
		 WHERE id = $1 AND status IN ('active', 'blocked');`, linkID, status)
}

// SetActiveOrganization changes the chat's منشأة and starts a new conversation,
// because a conversation belongs to one organisation.
func (t Tables) SetActiveOrganization(ctx context.Context, linkID int64, orgID *int64) error {
	return t.exec(ctx, `
		UPDATE {s}.links SET active_organization_id = $2, conversation_id = NULL, updated_at = now()
		 WHERE id = $1;`, linkID, orgID)
}

// SetConversation records the assistant conversation the chat continues.
func (t Tables) SetConversation(ctx context.Context, linkID int64, conversationID *int64) error {
	return t.exec(ctx, `UPDATE {s}.links SET conversation_id = $2, updated_at = now() WHERE id = $1;`,
		linkID, conversationID)
}

// SetMutedCategories replaces the muted notification categories.
func (t Tables) SetMutedCategories(ctx context.Context, linkID int64, muted []string) error {
	if muted == nil {
		muted = []string{}
	}
	return t.exec(ctx, `UPDATE {s}.links SET muted_categories = $2, updated_at = now() WHERE id = $1;`,
		linkID, muted)
}

// AcquireBusy takes the chat's one-question lock. The conditional UPDATE is
// atomic across server replicas.
func (t Tables) AcquireBusy(ctx context.Context, linkID int64, until time.Time) (bool, error) {
	var acquired bool
	err := t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, t.SQL(`
			UPDATE {s}.links SET busy_until = $2
			 WHERE id = $1 AND (busy_until IS NULL OR busy_until < now());`), linkID, until)
		acquired = err == nil && tag.RowsAffected() == 1
		return err
	})
	return acquired, err
}

// ReleaseBusy releases the lock.
func (t Tables) ReleaseBusy(ctx context.Context, linkID int64) error {
	return t.exec(ctx, `UPDATE {s}.links SET busy_until = NULL WHERE id = $1;`, linkID)
}

// Memberships lists the organisations a user is an active member of, with the
// same predicate the RBAC resolver uses to grant anything for one.
func (t Tables) Memberships(ctx context.Context, userID int64) ([]chatbridge.Membership, error) {
	var out []chatbridge.Membership
	err := t.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, `+orgNameSQL+`, o.type, o.status,
			       COALESCE(NULLIF(b.name->>'ar', ''), NULLIF(b.name->>'en', ''), '')
			  FROM org.members m
			  JOIN org.organizations o ON o.id = m.organization_id
			  LEFT JOIN org.branches b ON b.id = m.branch_id AND b.deleted_at IS NULL
			 WHERE m.user_id = $1
			   AND m.status = 'active' AND m.is_active = true
			   AND o.deleted_at IS NULL
			 ORDER BY o.id;`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m chatbridge.Membership
			if err := rows.Scan(&m.OrgID, &m.Name, &m.Type, &m.Status, &m.BranchName); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// OffersTopicEnabled reads the account-wide offers preference. No preferences
// row means the defaults, under which offers are on.
func (t Tables) OffersTopicEnabled(ctx context.Context, userID int64) (bool, error) {
	enabled := true
	err := t.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT NOT COALESCE(notification_topics->'offers' = 'false'::jsonb, false)
			  FROM profile.user_preferences WHERE user_id = $1;`, userID).Scan(&enabled)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return enabled, err
}

// NotificationCandidates returns in-app notifications for active links that
// have no recorded decision yet, created after the link was confirmed and not
// before since.
//
// There is deliberately no cursor. A notification row can commit after one
// with a higher id; a cursor would skip it for ever. NOT EXISTS over the
// decision table picks it up on the next claim, and the time bound keeps the
// scan to a recent window per linked user.
func (t Tables) NotificationCandidates(ctx context.Context, since time.Time, limit int) ([]chatbridge.Candidate, error) {
	var out []chatbridge.Candidate
	err := t.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, t.SQL(`
			SELECT l.id, k.id, COALESCE(k.active_organization_id, 0), k.muted_categories,
			       l.user_id, COALESCE(l.organization_id, 0), `+orgNameSQL+`,
			       l.title, l.body, l.required_permission
			  FROM {s}.links k
			  JOIN notifications.logs l
			    ON l.user_id = k.user_id
			   AND l.channel = 'in_app'
			   AND l.created_at >= GREATEST(k.confirmed_at, $1)
			  LEFT JOIN org.organizations o ON o.id = l.organization_id
			 WHERE k.status = 'active'
			   AND NOT EXISTS (
			       SELECT 1 FROM {s}.deliveries d
			        WHERE d.link_id = k.id AND d.notification_log_id = l.id)
			 ORDER BY l.id
			 LIMIT $2;`), since, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c chatbridge.Candidate
			if err := rows.Scan(&c.LogID, &c.LinkID, &c.LinkActiveOrgID, &c.MutedCategories,
				&c.UserID, &c.OrganizationID, &c.OrganizationName,
				&c.Title, &c.Body, &c.RequiredPermission); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// RecordDecisions writes one delivery row per decision. Two claims evaluating
// the same notification at once both try; the unique index keeps one.
func (t Tables) RecordDecisions(ctx context.Context, decisions []chatbridge.Decision) error {
	if len(decisions) == 0 {
		return nil
	}
	return t.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, d := range decisions {
			status := "queued"
			if d.DropReason != "" {
				status = "dropped"
			}
			batch.Queue(t.SQL(`
				INSERT INTO {s}.deliveries
				       (link_id, notification_log_id, kind, category, text, status, drop_reason)
				VALUES ($1, $2, 'notification', $3, $4, $5, $6)
				ON CONFLICT (link_id, notification_log_id) WHERE notification_log_id IS NOT NULL DO NOTHING;`),
				d.LinkID, d.LogID, string(d.Category), d.Text, status, d.DropReason)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// EnqueueSystemMessage queues a message the bridge itself writes.
func (t Tables) EnqueueSystemMessage(ctx context.Context, linkID int64, text string) error {
	return t.exec(ctx, `
		INSERT INTO {s}.deliveries (link_id, kind, category, text)
		VALUES ($1, 'system', 'general', $2);`, linkID, text)
}

// ExpireLeases runs inside a claim, before leasing: it fails deliveries whose
// lease lapsed maxLeaseAttempts times and drops notifications too old to send.
func (t Tables) ExpireLeases(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, t.SQL(`
		UPDATE {s}.deliveries
		   SET status = 'failed', last_error = 'lease expired without a report', lease_until = NULL
		 WHERE status = 'leased' AND lease_until < now() AND attempts >= $1;`), maxLeaseAttempts); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, t.SQL(`
		UPDATE {s}.deliveries SET status = 'dropped', drop_reason = 'expired', lease_until = NULL
		 WHERE status IN ('queued', 'leased') AND kind = 'notification'
		   AND created_at < now() - `+staleNotificationSQL+`
		   AND (status = 'queued' OR lease_until < now());`))
	return err
}

// DueDeliveriesSQL leases due rows for active links. FOR UPDATE SKIP LOCKED
// lets two overlapping n8n runs claim disjoint rows; a lease that expires
// without a report is claimable again. $1 is the limit, $2 the lease in
// seconds; the caller appends its RETURNING list, where d is the delivery and
// k its link.
func (t Tables) DueDeliveriesSQL() string {
	return t.SQL(`
		WITH due AS (
			SELECT d.id
			  FROM {s}.deliveries d
			  JOIN {s}.links k ON k.id = d.link_id AND k.status = 'active'
			 WHERE (d.status = 'queued' AND d.next_attempt_at <= now())
			    OR (d.status = 'leased' AND d.lease_until < now())
			 ORDER BY d.id
			 LIMIT $1
			   FOR UPDATE OF d SKIP LOCKED
		)
		UPDATE {s}.deliveries d
		   SET status = 'leased',
		       lease_until = now() + make_interval(secs => $2),
		       attempts = d.attempts + 1
		  FROM due, {s}.links k
		 WHERE d.id = due.id AND k.id = d.link_id
		RETURNING `)
}

// MarkDelivered records a successful send of a leased message.
func (t Tables) MarkDelivered(ctx context.Context, id int64) error {
	return t.exec(ctx, `
		UPDATE {s}.deliveries SET status = 'sent', sent_at = now(), lease_until = NULL, last_error = ''
		 WHERE id = $1 AND status = 'leased';`, id)
}

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
