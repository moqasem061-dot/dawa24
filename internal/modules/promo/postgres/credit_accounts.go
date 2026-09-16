package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Package accounts, one row per company.
//
// /admin/offers-packages listed packages, requests and ads — three views of the
// platform's supply. Nothing listed the demand side: which company holds which
// packages, what they have spent, what is left. Answering "why has this
// supplier run out?" meant reading promo.sponsorship_purchases by hand.

const creditAccountSQL = `
	SELECT p.organization_id,
	       COALESCE(NULLIF(o.trade_name->>'ar', ''), o.legal_name, '') AS org_name,
	       count(*)                                                    AS purchases,
	       count(*) FILTER (WHERE p.status = 'active'
	                          AND p.expires_at > now())                AS active_purchases,
	       COALESCE(sum(p.credits_total), 0)                           AS credits_total,
	       COALESCE(sum(p.credits_used), 0)                            AS credits_used,
	       COALESCE(sum(GREATEST(p.credits_total - p.credits_used, 0))
	                FILTER (WHERE p.status = 'active'
	                          AND p.expires_at > now()), 0)            AS credits_remaining,
	       max(p.created_at)                                           AS last_purchase_at
	FROM promo.sponsorship_purchases p
	JOIN org.organizations o ON o.id = p.organization_id AND o.deleted_at IS NULL`

// ListCreditAccounts returns one row per company that has ever bought a package.
//
// credits_remaining counts only live purchases: an expired package's unused
// credits are not spendable, and adding them to the total would tell an
// administrator a supplier has budget they cannot use.
func (r *Repository) ListCreditAccounts(
	ctx context.Context, search string, limit, offset int,
) ([]*promo.CreditAccount, int, error) {
	var (
		out   []*promo.CreditAccount
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const filter = `
			WHERE ($1 = '' OR o.trade_name->>'ar' ILIKE '%' || $1 || '%'
			              OR o.trade_name->>'en' ILIKE '%' || $1 || '%'
			              OR o.legal_name        ILIKE '%' || $1 || '%')`

		if err := tx.QueryRow(txCtx, `
			SELECT count(DISTINCT p.organization_id)
			FROM promo.sponsorship_purchases p
			JOIN org.organizations o ON o.id = p.organization_id AND o.deleted_at IS NULL`+
			filter+`;`, search).Scan(&total); err != nil {
			return fmt.Errorf("promo postgres: count credit accounts: %w", err)
		}

		rows, err := tx.Query(txCtx, creditAccountSQL+filter+`
			GROUP BY p.organization_id, org_name
			ORDER BY credits_remaining DESC, last_purchase_at DESC
			LIMIT $2 OFFSET $3;`, search, limit, offset)
		if err != nil {
			return fmt.Errorf("promo postgres: list credit accounts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var a promo.CreditAccount
			if err := rows.Scan(
				&a.OrganizationID, &a.OrganizationName, &a.Purchases, &a.ActivePurchases,
				&a.CreditsTotal, &a.CreditsUsed, &a.CreditsRemaining, &a.LastPurchaseAt,
			); err != nil {
				return fmt.Errorf("promo postgres: scan credit account: %w", err)
			}
			out = append(out, &a)
		}
		return rows.Err()
	})
	return out, total, err
}

// ListPurchasesForOrg returns a company's purchases newest first, for the
// administrator's drill-down. It reads as the system: the whole point of the
// screen is to look at someone else's account.
func (r *Repository) ListPurchasesForOrg(
	ctx context.Context, orgID int64, limit, offset int,
) ([]*promo.SponsorshipPurchase, int, error) {
	var (
		out   []*promo.SponsorshipPurchase
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM promo.sponsorship_purchases WHERE organization_id = $1;`,
			orgID,
		).Scan(&total); err != nil {
			return fmt.Errorf("promo postgres: count org purchases: %w", err)
		}

		rows, err := tx.Query(txCtx, `
			SELECT id, public_id, organization_id, package_id, credits_total, credits_used,
			       starts_at, expires_at, status, auto_renew, billing_cycle, amount,
			       payment_id, source_system, source_id, approved_by, approved_at,
			       created_at, updated_at
			FROM promo.sponsorship_purchases
			WHERE organization_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;`, orgID, limit, offset)
		if err != nil {
			return fmt.Errorf("promo postgres: list org purchases: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p promo.SponsorshipPurchase
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.PackageID, &p.CreditsTotal, &p.CreditsUsed,
				&p.StartsAt, &p.ExpiresAt, &p.Status, &p.AutoRenew, &p.BillingCycle, &p.Amount,
				&p.PaymentID, &p.SourceSystem, &p.SourceID, &p.ApprovedBy, &p.ApprovedAt,
				&p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return fmt.Errorf("promo postgres: scan org purchase: %w", err)
			}
			p.CreditsRemaining = p.CreditsRemainingInt()
			out = append(out, &p)
		}
		return rows.Err()
	})
	return out, total, err
}
