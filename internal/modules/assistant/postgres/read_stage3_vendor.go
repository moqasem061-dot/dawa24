package postgres

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readVendorProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionQuotaReport:
		return r.readProjectionRows(ctx, actor, `
			SELECT v.id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'quota_limit', v.quota_limit,
				'used', COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ol.organization_id=v.organization_id
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),
				'remaining', GREATEST(v.quota_limit-COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ol.organization_id=v.organization_id
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),0))
			  FROM catalog.product_variants v JOIN catalog.products p ON p.id=v.product_id
			  LEFT JOIN org.branches b ON b.id=v.branch_id
			 WHERE v.organization_id=$1 AND v.quota_limit IS NOT NULL AND v.quota_limit > 0
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY v.quota_limit ASC, v.id ASC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionImportRuns:
		return r.readProjectionRows(ctx, actor, `
			SELECT ci.id, jsonb_build_object('filename', ci.filename, 'phase', ci.phase,
				'status', ci.phase, 'total_rows', ci.total_rows, 'inserted_rows', ci.inserted_rows,
				'updated_rows', ci.updated_rows, 'error_rows', ci.error_rows, 'created_at', ci.created_at)
			  FROM ingest.catalog_imports ci WHERE ci.organization_id=$1
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR ci.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR ci.created_at <= $5)
			 ORDER BY ci.created_at DESC, ci.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionImportRunDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT ci.id, jsonb_build_object('filename', ci.filename, 'phase', ci.phase,
				'total_rows', ci.total_rows, 'inserted_rows', ci.inserted_rows,
				'updated_rows', ci.updated_rows, 'skipped_rows', ci.skipped_rows,
				'error_rows', ci.error_rows, 'matched_rows', ci.matched_rows,
				'review_rows', ci.review_rows, 'unmatched_rows', ci.unmatched_rows,
				'error_message', COALESCE(ci.error_message,''))
			  FROM ingest.catalog_imports ci WHERE ci.id=$1 AND ci.organization_id=$2`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionSponsorshipStatus:
		return r.readProjectionRows(ctx, actor, `
			SELECT sp.id, jsonb_build_object('status', sp.status, 'credits_total', sp.credits_total,
				'credits_used', sp.credits_used, 'remaining', sp.credits_total-sp.credits_used,
				'amount', sp.amount::text, 'starts_at', sp.starts_at, 'expires_at', sp.expires_at,
				'package', COALESCE(`+nameExpr("op.name")+`,''))
			  FROM promo.sponsorship_purchases sp LEFT JOIN promo.offer_packages op ON op.id=sp.package_id
			 WHERE sp.organization_id=$1 AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY sp.created_at DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionAccountProfile:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'organization_name', `+nameExpr("o.name")+`,
				'organization_number', COALESCE(o.organization_number, ''),
				'type', o.type, 'status', o.status,
				'tax_number', COALESCE(o.tax_number, ''),
				'email', COALESCE(o.email, ''), 'phone', COALESCE(o.phone, ''),
				'address', COALESCE(o.address, ''), 'rating', o.rating,
				'user_name', COALESCE(`+nameExpr("u.name")+`, u.email, ''),
				'user_role', COALESCE(u.role, ''), 'user_phone', COALESCE(u.phone, ''),
				'branches_count', (SELECT COUNT(*) FROM org.branches b WHERE b.organization_id = o.id AND b.deleted_at IS NULL),
				'registered_at', o.created_at)
			  FROM org.organizations o
			  LEFT JOIN identity.users u ON u.id = $2
			 WHERE o.id = $1 AND o.deleted_at IS NULL`, []any{orgID, actor.UserID}, 1, 0, false)
	case assistant.ProjectionInventoryHealth:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'total_tracked_items', (SELECT COUNT(*) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL),
				'out_of_stock_count', (SELECT COUNT(*) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL AND s.quantity <= 0),
				'low_stock_count', (SELECT COUNT(*) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL AND s.quantity > 0 AND s.quantity <= s.min_threshold),
				'healthy_stock_count', (SELECT COUNT(*) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL AND s.quantity > s.min_threshold),
				'total_stock_units', COALESCE((SELECT SUM(s.quantity) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL), 0),
				'estimated_valuation_egp', COALESCE((
					SELECT ROUND(SUM(s.quantity * COALESCE(v.price, 0)), 2)
					FROM inventory.stocks s
					LEFT JOIN catalog.product_variants v ON v.id = s.product_variant_id
					WHERE s.organization_id = $1 AND s.deleted_at IS NULL
				), 0)::text,
				'warehouses_count', (SELECT COUNT(DISTINCT s.warehouse_id) FROM inventory.stocks s WHERE s.organization_id = $1 AND s.deleted_at IS NULL))`, []any{orgID}, 1, 0, false)
	case assistant.ProjectionSalesInsights:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'revenue_last_30_days', COALESCE((
					SELECT SUM(sh.total_amount) FROM commerce.order_shipments sh
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '30 days'
					  AND sh.status NOT IN ('cancelled', 'returned', 'failed')
				), 0)::text,
				'revenue_prev_30_days', COALESCE((
					SELECT SUM(sh.total_amount) FROM commerce.order_shipments sh
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '60 days'
					  AND sh.created_at < now() - interval '30 days'
					  AND sh.status NOT IN ('cancelled', 'returned', 'failed')
				), 0)::text,
				'shipments_last_30_days', (
					SELECT COUNT(*) FROM commerce.order_shipments sh
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '30 days'
				),
				'delivered_shipments_30d', (
					SELECT COUNT(*) FROM commerce.order_shipments sh
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '30 days'
					  AND sh.status = 'delivered'
				),
				'active_buyers_count_30d', (
					SELECT COUNT(DISTINCT o.organization_id)
					FROM commerce.order_shipments sh
					JOIN commerce.orders o ON o.id = sh.order_id
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '30 days'
				),
				'average_shipment_value', COALESCE((
					SELECT ROUND(AVG(sh.total_amount), 2) FROM commerce.order_shipments sh
					WHERE sh.organization_id = $1
					  AND sh.created_at >= now() - interval '30 days'
					  AND sh.status NOT IN ('cancelled', 'returned')
				), 0)::text,
				'currency', 'EGP')`, []any{orgID}, 1, 0, false)
	default:
		return r.readVendorProjectionExt(ctx, actor, q)
	}
}
