package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readVendorProjectionExt(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionBatchExpiryReport:
		return r.readProjectionRows(ctx, actor, `
			SELECT v.id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`,
				'variant', COALESCE(`+nameExpr("v.name")+`,''),
				'sku', COALESCE(v.sku,''),
				'batch_number', COALESCE(v.batch_number,''),
				'expiry_date', v.expiry_date,
				'days_until_expiry', CASE WHEN v.expiry_date IS NOT NULL THEN (v.expiry_date - CURRENT_DATE) ELSE NULL END,
				'is_near_expiry', CASE WHEN v.expiry_date IS NOT NULL AND v.expiry_date <= (CURRENT_DATE + INTERVAL '90 days') THEN true ELSE false END,
				'stock', COALESCE((SELECT SUM(s.quantity) FROM inventory.stocks s WHERE s.product_variant_id = v.id AND s.deleted_at IS NULL), 0))
			  FROM catalog.product_variants v
			  JOIN catalog.products p ON p.id = v.product_id
			 WHERE v.organization_id = $1 AND v.deleted_at IS NULL
			   AND ($2 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%' OR COALESCE(v.batch_number,'') ILIKE '%' || $2 || '%')
			   AND ($3 = '' OR ($3 = 'near_expiry' AND v.expiry_date <= (CURRENT_DATE + INTERVAL '90 days')))
			   AND ($4::timestamptz IS NULL OR v.expiry_date >= $4::timestamptz::date)
			   AND ($5::timestamptz IS NULL OR v.expiry_date <= $5::timestamptz::date)
			 ORDER BY v.expiry_date ASC NULLS LAST, v.id ASC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionDispatchSchedule:
		return r.readProjectionRows(ctx, actor, `
			SELECT sh.id, jsonb_build_object(
				'shipment_number', sh.shipment_number,
				'order_number', COALESCE(o.order_number,''),
				'buyer', COALESCE(`+nameExpr("buy.name")+`,''),
				'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'city', COALESCE(b.address,''),
				'total_amount', sh.total_amount::text,
				'status', sh.status,
				'placed_at', sh.created_at)
			  FROM commerce.order_shipments sh
			  JOIN commerce.orders o ON o.id = sh.order_id
			  LEFT JOIN org.organizations buy ON buy.id = o.organization_id
			  LEFT JOIN org.branches b ON b.id = o.branch_id
			 WHERE sh.organization_id = $1 AND o.deleted_at IS NULL
			   -- What ships next: open shipments unless a status is asked for.
			   AND (($3 = '' AND sh.status IN ('pending','processing','confirmed','on_hold','shipped','in_transit','out_for_delivery'))
			        OR sh.status = $3)
			   AND ($2 = '' OR sh.shipment_number ILIKE '%' || $2 || '%' OR o.order_number ILIKE '%' || $2 || '%' OR `+nameExpr("buy.name")+` ILIKE '%' || $2 || '%')
			   AND ($4::timestamptz IS NULL OR sh.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR sh.created_at <= $5)
			 ORDER BY sh.created_at ASC, sh.id ASC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported vendor projection %q", q.Kind)
	}
}
