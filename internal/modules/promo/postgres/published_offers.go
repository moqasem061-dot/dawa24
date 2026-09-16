package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// publishedAtExpr is when buyers could first see an offer: it was created, then
// approved, then its start date arrived. An old draft approved today is new to
// buyers today.
const publishedAtExpr = `GREATEST(o.created_at, COALESCE(o.approved_at, o.created_at), COALESCE(o.starts_at, o.created_at))`

// ListPublishedOffers returns offers that became visible since `since` and are
// still live: the buyer rule's live, supplier and branch predicates, plus at
// least one product, because an offer without products cannot be ordered.
//
// Cities are the offer's own approved location rules when it has any, and
// otherwise the supplier's active weekly coverage for the offer's branch —
// the same precedence the buyer coverage rule applies.
func (r *Repository) ListPublishedOffers(ctx context.Context, since time.Time, limit int) ([]*promo.PublishedOffer, error) {
	var rule offerRule
	sinceArg := rule.bind(since)
	limitArg := rule.bind(limit)
	query := `
		SELECT o.id, COALESCE(o.title->>'ar', ''), COALESCE(o.title->>'en', ''), COALESCE(o.description->>'ar', ''),
		       COALESCE(o.discount_type, ''), COALESCE(o.discount_value, 0)::float8,
		       COALESCE(o.total_price, 0)::float8, COALESCE(o.min_order_amount, 0)::float8,
		       COALESCE(NULLIF(so.trade_name->>'ar', ''), NULLIF(so.name->>'ar', ''), so.legal_name, ''),
		       ` + publishedAtExpr + `, o.expires_at,
		       CASE WHEN EXISTS (SELECT 1 FROM promo.offer_location_covers l
		                          WHERE l.offer_id = o.id AND l.status = 'active' AND l.admin_status = 'approved')
		            THEN ARRAY(SELECT DISTINCT COALESCE(c.name->>'ar', c.name->>'en')
		                         FROM promo.offer_location_covers l
		                         JOIN platform_admin.cities c ON c.id = l.city_id
		                        WHERE l.offer_id = o.id AND l.status = 'active' AND l.admin_status = 'approved'
		                          AND COALESCE(c.name->>'ar', c.name->>'en') IS NOT NULL)
		            ELSE ARRAY(SELECT DISTINCT COALESCE(c.name->>'ar', g.name->>'ar', c.name->>'en', g.name->>'en')
		                         FROM workflow.weekly_coverages wc
		                         LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
		                         LEFT JOIN platform_admin.governorates g ON g.id = wc.governorate_id
		                         LEFT JOIN org.branches b ON b.id = wc.branch_id
		                        WHERE wc.organization_id = o.organization_id AND wc.is_active
		                          AND COALESCE(c.name->>'ar', g.name->>'ar', c.name->>'en', g.name->>'en') IS NOT NULL
		                          AND (wc.branch_id IS NULL OR (b.deleted_at IS NULL AND b.status <> 'inactive'))
		                          AND (o.branch_id IS NULL OR wc.branch_id IS NULL OR wc.branch_id = o.branch_id))
		       END,
		       ARRAY(SELECT COALESCE(p.name->>'ar', p.name->>'en', '')
		               FROM promo.offer_products op
		               JOIN catalog.products p ON p.id = op.product_id AND p.deleted_at IS NULL
		              WHERE op.offer_id = o.id ORDER BY op.id LIMIT 5)
		  FROM promo.offers o
		  JOIN org.organizations so ON so.id = o.organization_id
		 WHERE ` + rule.live() + ` AND ` + rule.supplier() + ` AND ` + rule.branch() + `
		   AND EXISTS (SELECT 1 FROM promo.offer_products op WHERE op.offer_id = o.id)
		   AND ` + publishedAtExpr + ` >= ` + sinceArg + `
		 ORDER BY ` + publishedAtExpr + ` DESC, o.id DESC
		 LIMIT ` + limitArg

	var out []*promo.PublishedOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, query, rule.args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p promo.PublishedOffer
			if err := rows.Scan(&p.ID, &p.TitleAr, &p.TitleEn, &p.DescriptionAr,
				&p.DiscountType, &p.DiscountValue, &p.TotalPrice, &p.MinOrderAmount,
				&p.SupplierName, &p.PublishedAt, &p.ExpiresAt, &p.Cities, &p.Products); err != nil {
				return err
			}
			p.CitiesTotal = len(p.Cities)
			if len(p.Cities) > promo.MaxAnnouncedCities {
				p.Cities = p.Cities[:promo.MaxAnnouncedCities]
			}
			out = append(out, &p)
		}
		return rows.Err()
	})
	return out, err
}
