package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The subscriber log's query.
//
// One statement joins the organisation, the plan and the amount actually paid,
// so the screen renders names instead of the ids it used to. The count runs
// over the same predicate as the page, which is the only way a pager can be
// trusted.

// adminSubscriptionFrom is the shared FROM/JOIN chain. The count and the page
// must filter over identical relations or they disagree the moment a filter
// touches a joined column.
const adminSubscriptionFrom = `
	FROM billing.subscriptions s
	LEFT JOIN org.organizations o ON o.id = s.organization_id
	LEFT JOIN identity.users u ON u.id = s.user_id
	LEFT JOIN billing.plans p ON p.id = s.plan_id`

// AdminListSubscriptionRows returns one filtered page of the subscriber log.
func (r *Repository) AdminListSubscriptionRows(
	ctx context.Context, f billing.AdminSubscriptionFilter,
) ([]*billing.AdminSubscriptionRow, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := make([]any, 0, 12)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.OrganizationQuery != "" {
		p := arg("%" + f.OrganizationQuery + "%")
		// The buyer may be an organisation or a lone user, so the search covers
		// both. Without the user arm, a personal subscription is unfindable by
		// any name at all.
		where = append(where, fmt.Sprintf(`(
			o.name->>'ar' ILIKE %[1]s OR o.name->>'en' ILIKE %[1]s
			OR o.trade_name->>'ar' ILIKE %[1]s OR o.trade_name->>'en' ILIKE %[1]s
			OR o.legal_name ILIKE %[1]s
			OR u.name->>'ar' ILIKE %[1]s OR u.name->>'en' ILIKE %[1]s
			OR u.email::text ILIKE %[1]s)`, p))
	}
	if f.PlanID > 0 {
		where = append(where, "s.plan_id = "+arg(f.PlanID))
	}
	if f.Status != "" {
		where = append(where, "s.status = "+arg(f.Status))
	}
	if f.BillingCycle != "" {
		where = append(where, "COALESCE(s.billing_cycle, 'monthly') = "+arg(f.BillingCycle))
	}
	if f.AutoRenew != nil {
		where = append(where, "COALESCE(s.auto_renew, false) = "+arg(*f.AutoRenew))
	}
	if f.ExpiringWithinDays > 0 {
		// Not yet expired AND expiring inside the window: an operator asking
		// "what lapses this month" does not want what lapsed last month, which
		// the status filter answers instead.
		where = append(where, fmt.Sprintf(
			"s.expires_at > now() AND s.expires_at <= now() + (%s || ' days')::interval",
			arg(fmt.Sprintf("%d", f.ExpiringWithinDays))))
	}
	if f.StartsFrom != nil {
		where = append(where, "s.starts_at >= "+arg(*f.StartsFrom))
	}
	if f.StartsTo != nil {
		where = append(where, "s.starts_at < "+arg(*f.StartsTo))
	}
	if f.ExpiresFrom != nil {
		where = append(where, "s.expires_at >= "+arg(*f.ExpiresFrom))
	}
	if f.ExpiresTo != nil {
		where = append(where, "s.expires_at < "+arg(*f.ExpiresTo))
	}

	whereSQL := strings.Join(where, " AND ")

	var out []*billing.AdminSubscriptionRow
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) `+adminSubscriptionFrom+` WHERE `+whereSQL, args...,
		).Scan(&total); err != nil {
			return fmt.Errorf("count subscriptions: %w", err)
		}
		if total == 0 {
			return nil
		}

		paged := append(append([]any{}, args...), f.Limit, f.Offset)
		query := fmt.Sprintf(`
			SELECT s.id, s.public_id, s.user_id, s.organization_id, s.plan_id, s.status,
			       COALESCE(s.billing_cycle, 'monthly'), COALESCE(s.auto_renew, false),
			       s.starts_at, s.expires_at, s.last_renewed_at,
			       COALESCE(s.renewal_attempts, 0), COALESCE(s.source_system, ''), s.source_id,
			       s.created_at, s.updated_at,
			       COALESCE(NULLIF(o.trade_name, '{}'::jsonb), o.name, '{}'::jsonb),
			       COALESCE(o.type, ''),
			       COALESCE(u.name, '{}'::jsonb), COALESCE(u.email::text, ''),
			       COALESCE(p.name, '{}'::jsonb), COALESCE(p.slug, ''),
			       COALESCE(p.is_default, false) OR COALESCE(p.price_month, 0) = 0,
			       COALESCE((
			           SELECT h.amount_minor FROM billing.subscription_histories h
			           WHERE h.subscription_id = s.id AND h.amount_minor > 0
			           ORDER BY h.created_at DESC LIMIT 1), 0),
			       COALESCE((
			           SELECT count(*) FROM billing.subscription_histories h
			           WHERE h.subscription_id = s.id), 0)
			%s
			WHERE %s
			ORDER BY s.starts_at DESC, s.id DESC
			LIMIT $%d OFFSET $%d`,
			adminSubscriptionFrom, whereSQL, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, query, paged...)
		if err != nil {
			return fmt.Errorf("list subscriptions: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row billing.AdminSubscriptionRow
			var status string
			var amountMinor int64
			if err := rows.Scan(
				&row.ID, &row.PublicID, &row.UserID, &row.OrganizationID, &row.PlanID, &status,
				&row.BillingCycle, &row.AutoRenew,
				&row.StartsAt, &row.ExpiresAt, &row.LastRenewedAt,
				&row.RenewalAttempts, &row.SourceSystem, &row.SourceID,
				&row.CreatedAt, &row.UpdatedAt,
				&row.OrganizationName, &row.OrganizationType,
				&row.UserName, &row.UserEmail,
				&row.PlanName, &row.PlanSlug, &row.PlanIsFree,
				&amountMinor, &row.HistoryRows,
			); err != nil {
				return fmt.Errorf("scan subscription row: %w", err)
			}
			row.Status = billing.SubscriptionStatus(status)
			row.AmountPaid = money.FromMinor(amountMinor)
			out = append(out, &row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("billing postgres: admin subscription rows: %w", err)
	}
	return out, total, nil
}

// AdminSubscriptionHistory returns the upgrade, downgrade and renewal trail for
// one subscription, newest first.
func (r *Repository) AdminSubscriptionHistory(
	ctx context.Context, subscriptionID int64,
) ([]*billing.SubscriptionHistoryRow, error) {
	if subscriptionID <= 0 {
		return nil, nil
	}
	var out []*billing.SubscriptionHistoryRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT h.id, h.subscription_id, h.organization_id, h.user_id, h.plan_id,
			       COALESCE(h.action, ''), COALESCE(h.amount_minor, 0),
			       COALESCE(h.currency, 'EGP'), COALESCE(h.details, ''), h.created_at,
			       COALESCE(p.name, '{}'::jsonb)
			FROM billing.subscription_histories h
			LEFT JOIN billing.plans p ON p.id = h.plan_id
			WHERE h.subscription_id = $1
			ORDER BY h.created_at DESC, h.id DESC
			LIMIT 200;`, subscriptionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h billing.SubscriptionHistoryRow
			var amountMinor int64
			if err := rows.Scan(&h.ID, &h.SubscriptionID, &h.OrganizationID, &h.UserID,
				&h.PlanID, &h.Action, &amountMinor, &h.Currency, &h.Details,
				&h.CreatedAt, &h.PlanName); err != nil {
				return err
			}
			h.Amount = money.FromMinor(amountMinor)
			out = append(out, &h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("billing postgres: subscription history: %w", err)
	}
	return out, nil
}

// RecordSubscriptionHistory files one entry of a subscription's trail.
//
// Only the renewal path used to write here, and it discarded its error, so the
// trail an administrator reads was empty for every first purchase and every
// plan change -- the two transitions anyone actually asks about. Every
// transition writes one row now, and a failure is returned rather than
// swallowed: a subscription that activated without leaving a record is a
// reconciliation problem later, and the caller is the only one who can decide
// whether that is fatal.
func (r *Repository) RecordSubscriptionHistory(
	ctx context.Context, h billing.SubscriptionHistoryEntry,
) error {
	if h.SubscriptionID <= 0 {
		return nil
	}
	currency := h.Currency
	if currency == "" {
		currency = "EGP"
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO billing.subscription_histories (
				subscription_id, organization_id, user_id, plan_id,
				action, amount_minor, currency, details
			) VALUES ($1, NULLIF($2, 0), NULLIF($3, 0), NULLIF($4, 0), $5, $6, $7, $8);`,
			h.SubscriptionID, h.OrganizationID, h.UserID, h.PlanID,
			h.Action, h.AmountMinor, currency, h.Details)
		if err != nil {
			return fmt.Errorf("billing postgres: record subscription history: %w", err)
		}
		return nil
	})
}
