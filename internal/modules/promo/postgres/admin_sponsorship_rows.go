package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Product sponsorship, joined once.
//
// The screen this serves used to read five hundred requests and then resolve
// the product, the sponsor and the package one id at a time. Everything below
// is one statement, and the two performance columns come from the tables that
// record them rather than from a placeholder.

const adminSponsorshipFrom = `
	FROM promo.sponsorship_requests sr
	LEFT JOIN org.organizations o ON o.id = sr.organization_id
	LEFT JOIN catalog.product_variants pv ON pv.id = sr.item_id
	LEFT JOIN catalog.products p ON (p.id = sr.item_id OR p.id = pv.product_id)
	LEFT JOIN promo.offer_packages pk ON pk.id = sr.package_id
	LEFT JOIN promo.sponsorship_purchases pu ON pu.id = sr.purchase_id`

// adminSponsorshipBase is the predicate every listing and count shares: product
// sponsorships only. A request with no item_type is one by default, which is
// how the rows created before the column existed are still counted.
const adminSponsorshipBase = `(sr.item_type = 'product' OR COALESCE(sr.item_type, '') = '')`

// adminSponsorshipTabSQL maps a tab onto its predicate.
//
// The three states overlap in the data — a rejected request can also be past
// its expiry — so the order here is the order the screen reads them in:
// rejected first, then expired, then pending, then active. Without that a
// rejected sponsorship would be counted twice and the badges would not sum.
func adminSponsorshipTabSQL(tab string) string {
	switch tab {
	case "rejected":
		return `(sr.admin_status = 'rejected' OR sr.status = 'rejected')`
	case "expired":
		return `(sr.admin_status <> 'rejected' AND sr.status <> 'rejected'
		         AND (sr.status = 'expired' OR (sr.expires_at IS NOT NULL AND sr.expires_at < now())))`
	case "pending":
		return `(sr.admin_status = 'pending'
		         AND sr.status <> 'rejected'
		         AND (sr.expires_at IS NULL OR sr.expires_at >= now()))`
	case "active":
		return `((sr.admin_status = 'approved' OR sr.status = 'active')
		         AND sr.admin_status <> 'rejected' AND sr.status <> 'rejected'
		         AND sr.status <> 'expired'
		         AND (sr.expires_at IS NULL OR sr.expires_at >= now()))`
	default:
		return "TRUE"
	}
}

// adminSponsorshipWhere builds the shared filter, minus the tab.
func adminSponsorshipWhere(f promo.AdminSponsorshipFilter) (string, []any) {
	where := []string{adminSponsorshipBase}
	args := make([]any, 0, 8)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.OrganizationID > 0 {
		where = append(where, "sr.organization_id = "+arg(f.OrganizationID))
	}
	if f.PackageID > 0 {
		where = append(where, "sr.package_id = "+arg(f.PackageID))
	}
	if f.TierLevel > 0 {
		where = append(where, "COALESCE(pk.tier_level, 0) = "+arg(f.TierLevel))
	}
	if f.StartsFrom != nil {
		where = append(where, "sr.starts_at >= "+arg(*f.StartsFrom))
	}
	if f.StartsTo != nil {
		where = append(where, "sr.starts_at < "+arg(*f.StartsTo))
	}
	if f.Search != "" {
		p := arg("%" + f.Search + "%")
		where = append(where, fmt.Sprintf(`(
			p.name->>'ar' ILIKE %[1]s OR p.name->>'en' ILIKE %[1]s
			OR COALESCE(p.sku, '') ILIKE %[1]s OR COALESCE(p.barcode, '') ILIKE %[1]s
			OR COALESCE(o.legal_name, '') ILIKE %[1]s
			OR o.trade_name->>'ar' ILIKE %[1]s OR o.name->>'ar' ILIKE %[1]s
			OR pk.name->>'ar' ILIKE %[1]s OR pk.name->>'en' ILIKE %[1]s)`, p))
	}
	return strings.Join(where, " AND "), args
}

// ListAdminSponsorshipRows returns one filtered page of product sponsorships.
func (r *Repository) ListAdminSponsorshipRows(
	ctx context.Context, f promo.AdminSponsorshipFilter,
) ([]*promo.AdminSponsorshipRow, int, error) {
	f.Normalize()

	whereSQL, args := adminSponsorshipWhere(f)
	whereSQL += " AND " + adminSponsorshipTabSQL(f.Tab)

	var out []*promo.AdminSponsorshipRow
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) `+adminSponsorshipFrom+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("count sponsorships: %w", err)
		}
		if total == 0 {
			return nil
		}

		paged := append(append([]any{}, args...), f.Limit, f.Offset)
		query := fmt.Sprintf(`
			SELECT sr.id, sr.public_id, sr.organization_id, sr.purchase_id,
			       COALESCE(NULLIF(o.trade_name, '{}'::jsonb), o.name, '{}'::jsonb),
			       COALESCE(o.type, ''),
			       COALESCE(p.id, pv.product_id, sr.item_id, 0), COALESCE(p.name, '{}'::jsonb),
			       COALESCE(NULLIF(p.sku, ''), pv.sku, ''), COALESCE(NULLIF(p.image, ''), p.image_link, ''),
			       COALESCE(sr.item_id, 0),
			       COALESCE(sr.package_id, 0), COALESCE(pk.name, '{}'::jsonb),
			       COALESCE(pk.tier_level, 0),
			       COALESCE(sr.credits_used, 0), COALESCE(pu.credits_total, 0),
			       COALESCE(pu.amount, 0),
			       COALESCE(sr.admin_status, ''), COALESCE(sr.status, ''),
			       COALESCE(sr.admin_notes, ''), sr.reviewed_by, sr.reviewed_at,
			       sr.starts_at, sr.expires_at, sr.created_at,
			       COALESCE(imp.n, 0), COALESCE(clk.n, 0)
			%s
			LEFT JOIN LATERAL (
				SELECT count(*) AS n FROM promo.ad_impressions ai
				WHERE ai.ad_id = sr.item_id
			) imp ON true
			LEFT JOIN LATERAL (
				SELECT count(*) AS n FROM promo.offer_clicks oc
				WHERE oc.offer_id = sr.item_id
			) clk ON true
			WHERE %s
			ORDER BY sr.created_at DESC, sr.id DESC
			LIMIT $%d OFFSET $%d`,
			adminSponsorshipFrom, whereSQL, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, query, paged...)
		if err != nil {
			return fmt.Errorf("list sponsorships: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			row, scanErr := scanAdminSponsorshipRow(rows)
			if scanErr != nil {
				return scanErr
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("promo postgres: admin sponsorship rows: %w", err)
	}
	return out, total, nil
}

// AdminSponsorshipCounts counts each tab over the whole filtered set.
//
// One statement with four FILTER clauses rather than five round trips, and the
// same predicates the tabs themselves use, so a badge cannot disagree with the
// list it labels.
func (r *Repository) AdminSponsorshipCounts(
	ctx context.Context, f promo.AdminSponsorshipFilter,
) (promo.AdminSponsorshipCounts, error) {
	f.Normalize()
	f.Tab = "all"
	whereSQL, args := adminSponsorshipWhere(f)

	var c promo.AdminSponsorshipCounts
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := fmt.Sprintf(`
			SELECT count(*),
			       count(*) FILTER (WHERE %s),
			       count(*) FILTER (WHERE %s),
			       count(*) FILTER (WHERE %s),
			       count(*) FILTER (WHERE %s)
			%s
			WHERE %s`,
			adminSponsorshipTabSQL("active"),
			adminSponsorshipTabSQL("pending"),
			adminSponsorshipTabSQL("expired"),
			adminSponsorshipTabSQL("rejected"),
			adminSponsorshipFrom, whereSQL)
		return tx.QueryRow(txCtx, query, args...).Scan(
			&c.Total, &c.Active, &c.Pending, &c.Expired, &c.Rejected)
	})
	if err != nil {
		return promo.AdminSponsorshipCounts{}, fmt.Errorf("promo postgres: sponsorship counts: %w", err)
	}
	return c, nil
}

// GetAdminSponsorshipRow returns one sponsorship for its detail page.
func (r *Repository) GetAdminSponsorshipRow(
	ctx context.Context, id int64,
) (*promo.AdminSponsorshipRow, error) {
	if id <= 0 {
		return nil, nil
	}
	var out *promo.AdminSponsorshipRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT sr.id, sr.public_id, sr.organization_id, sr.purchase_id,
			       COALESCE(NULLIF(o.trade_name, '{}'::jsonb), o.name, '{}'::jsonb),
			       COALESCE(o.type, ''),
			       COALESCE(p.id, pv.product_id, sr.item_id, 0), COALESCE(p.name, '{}'::jsonb),
			       COALESCE(NULLIF(p.sku, ''), pv.sku, ''), COALESCE(NULLIF(p.image, ''), p.image_link, ''),
			       COALESCE(sr.item_id, 0),
			       COALESCE(sr.package_id, 0), COALESCE(pk.name, '{}'::jsonb),
			       COALESCE(pk.tier_level, 0),
			       COALESCE(sr.credits_used, 0), COALESCE(pu.credits_total, 0),
			       COALESCE(pu.amount, 0),
			       COALESCE(sr.admin_status, ''), COALESCE(sr.status, ''),
			       COALESCE(sr.admin_notes, ''), sr.reviewed_by, sr.reviewed_at,
			       sr.starts_at, sr.expires_at, sr.created_at,
			       COALESCE((SELECT count(*) FROM promo.ad_impressions ai WHERE ai.ad_id = sr.item_id), 0),
			       COALESCE((SELECT count(*) FROM promo.offer_clicks oc WHERE oc.offer_id = sr.item_id), 0)
			` + adminSponsorshipFrom + `
			WHERE sr.id = $1`
		rows, err := tx.Query(txCtx, query, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return rows.Err()
		}
		row, scanErr := scanAdminSponsorshipRow(rows)
		if scanErr != nil {
			return scanErr
		}
		out = row
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("promo postgres: get sponsorship: %w", err)
	}
	return out, nil
}

// scanAdminSponsorshipRow keeps the listing and the detail on one projection,
// so a column added to one cannot be forgotten by the other.
func scanAdminSponsorshipRow(rows pgx.Rows) (*promo.AdminSponsorshipRow, error) {
	var row promo.AdminSponsorshipRow
	var orgID *int64
	if err := rows.Scan(
		&row.ID, &row.PublicID, &orgID, &row.PurchaseID,
		&row.OrganizationName, &row.OrganizationType,
		&row.ProductID, &row.ProductName, &row.ProductSKU, &row.ProductImage,
		&row.ItemID,
		&row.PackageID, &row.PackageName, &row.TierLevel,
		&row.CreditsUsed, &row.CreditsTotal, &row.Amount,
		&row.AdminStatus, &row.Status, &row.AdminNotes,
		&row.ReviewedBy, &row.ReviewedAt,
		&row.StartsAt, &row.ExpiresAt, &row.CreatedAt,
		&row.Impressions, &row.Clicks,
	); err != nil {
		return nil, fmt.Errorf("scan sponsorship row: %w", err)
	}
	if orgID != nil {
		row.OrganizationID = *orgID
	}
	return &row, nil
}

// GetAdminSponsorshipCreditEntries returns credit ledger entries for this request or its purchase.
func (r *Repository) GetAdminSponsorshipCreditEntries(
	ctx context.Context, purchaseID, requestID int64,
) ([]*promo.CreditEntry, error) {
	var out []*promo.CreditEntry
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, purchase_id, delta, balance_after,
			       reason, entity_type, entity_id, actor_user_id, note, created_at
			FROM promo.sponsorship_credit_entries
			WHERE (entity_id = $1 AND entity_type = 'product')
			   OR (purchase_id > 0 AND purchase_id = $2)
			ORDER BY created_at DESC, id DESC
			LIMIT 50;`
		rows, err := tx.Query(txCtx, query, requestID, purchaseID)
		if err != nil {
			return fmt.Errorf("query credit entries: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var e promo.CreditEntry
			if err := rows.Scan(
				&e.ID, &e.PublicID, &e.OrganizationID, &e.PurchaseID, &e.Delta, &e.BalanceAfter,
				&e.Reason, &e.EntityType, &e.EntityID, &e.ActorUserID, &e.Note, &e.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, &e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("promo postgres: sponsorship credit entries: %w", err)
	}
	return out, nil
}

// GetAdminSponsorshipEvents returns impression and click events for this item.
func (r *Repository) GetAdminSponsorshipEvents(
	ctx context.Context, itemID int64,
) ([]*promo.AdminSponsorshipEvent, error) {
	var out []*promo.AdminSponsorshipEvent
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 'impression' AS event_type, id, user_id, COALESCE(ip_address, ''), COALESCE(user_agent, ''), created_at
			FROM promo.ad_impressions
			WHERE ad_id = $1
			UNION ALL
			SELECT 'click' AS event_type, id, user_id, COALESCE(ip_address, ''), '', created_at
			FROM promo.offer_clicks
			WHERE offer_id = $1
			ORDER BY created_at DESC
			LIMIT 50;`
		rows, err := tx.Query(txCtx, query, itemID)
		if err != nil {
			return fmt.Errorf("query sponsorship events: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var ev promo.AdminSponsorshipEvent
			if err := rows.Scan(
				&ev.EventType, &ev.ID, &ev.UserID, &ev.IPAddress, &ev.Detail, &ev.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, &ev)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("promo postgres: sponsorship events: %w", err)
	}
	return out, nil
}
