package postgres

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readPharmacyProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionReorderSuggestions:
		return r.readProjectionRows(ctx, actor, `
			SELECT COALESCE(l.product_id, 0), jsonb_build_object(
				'product', COALESCE(NULLIF(`+nameExpr("p.name")+`, ''), NULLIF(`+nameExpr("l.product_name")+`, ''), 'صنف'),
				'quantity', COALESCE(SUM(l.quantity),0),
				'orders', COUNT(DISTINCT o.id),
				'last_purchased_at', MAX(o.created_at))
			  FROM commerce.order_lines l
			  JOIN commerce.orders o ON o.id = l.order_id
			  LEFT JOIN catalog.products p ON p.id = l.product_id
			 WHERE o.organization_id = $1 AND o.deleted_at IS NULL
			   AND ($2 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%' OR `+nameExpr("l.product_name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			   AND ($4::timestamptz IS NULL OR o.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR o.created_at <= $5)
			 GROUP BY l.product_id, `+nameExpr("p.name")+`, `+nameExpr("l.product_name")+`
			 ORDER BY MAX(o.created_at) ASC, SUM(l.quantity) DESC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSavingProducts:
		return r.readProjectionRows(ctx, actor, `
			SELECT sp.product_id, jsonb_build_object(
				'product', COALESCE(sp.name_product, `+nameExpr("p.name")+`),
				'sku', COALESCE(sp.sku,''), 'quantity', sp.qty, 'price', sp.price::text,
				'created_at', sp.created_at)
			  FROM catalog.saving_products sp LEFT JOIN catalog.products p ON p.id = sp.product_id
			 WHERE sp.organization_id = $1 AND sp.deleted_at IS NULL
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY sp.created_at DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSmartOrderDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT sr.id, jsonb_build_object(
				'run_number', sr.run_number, 'status', sr.status, 'filename', sr.original_filename,
				'total_rows', sr.total_rows, 'matched_rows', sr.matched_rows,
				'unmatched_rows', sr.unmatched_rows, 'coverage_blocked_rows', sr.coverage_blocked_rows,
				'quota_blocked_rows', sr.quota_blocked_rows, 'estimated_total', sr.estimated_total::text,
				'failure_reason', COALESCE(sr.failure_reason,''),
				'lines', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'row', sl.row_number, 'name', sl.raw_name, 'outcome', sl.outcome,
					'reason', sl.outcome_reason, 'quantity', sl.effective_qty))
					FROM smartorder.run_lines sl WHERE sl.run_id = sr.id), '[]'::jsonb))
			  FROM smartorder.runs sr WHERE sr.id = $1 AND sr.organization_id = $2`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionDecisionMemory:
		return r.readProjectionRows(ctx, actor, `
			SELECT md.id, jsonb_build_object(
				'key', md.decision_key, 'name', md.norm_name, 'product',
				COALESCE(`+nameExpr("p.name")+`,''), 'confidence', md.confidence::text,
				'reason', COALESCE(md.reason,''), 'hits', md.hit_count,
				'last_used_at', md.last_used_at)
			  FROM catalog.match_decisions md LEFT JOIN catalog.products p ON p.id = md.chosen_product_id
			 WHERE (md.organization_id = $1 OR md.scope = 'platform')
			   AND ($2 = '' OR md.norm_name ILIKE '%' || $2 || '%' OR md.decision_key ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY md.last_used_at DESC NULLS LAST, md.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSupplierProfile:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'name', `+nameExpr("o.name")+`, 'organization_number', o.organization_number,
				'type', o.type, 'status', o.status, 'rating', o.rating,
				'phone', COALESCE(o.phone,''), 'email', COALESCE(o.email,''),
				'address', COALESCE(o.address,''), 'branches',
				 (SELECT COUNT(*) FROM org.branches b WHERE b.organization_id=o.id AND b.deleted_at IS NULL))
			  FROM org.organizations o
			 WHERE o.type IN ('vendor','supplier','company','agency') AND o.deleted_at IS NULL
			   AND o.status = 'approved' AND o.id <> $1
			   AND $1::bigint > 0 AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			   AND ($2 = '' OR `+nameExpr("o.name")+` ILIKE '%' || $2 || '%' OR o.organization_number ILIKE '%' || $2 || '%')
			 ORDER BY o.rating DESC NULLS LAST, o.id ASC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionFavourites:
		return r.readProjectionRows(ctx, actor, `
			SELECT uf.product_id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'sku', COALESCE(p.sku,''), 'saved_at', uf.created_at)
			  FROM identity.user_favorites uf JOIN catalog.products p ON p.id = uf.product_id
			 WHERE uf.user_id = $8 AND p.deleted_at IS NULL
			   AND $1::bigint > 0 AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY uf.created_at DESC LIMIT $6 OFFSET $7`, append(args, actor.UserID), q.Limit, q.Offset, false)
	case assistant.ProjectionNotifications:
		return r.readProjectionRows(ctx, actor, `
			SELECT n.id, jsonb_build_object(
				'title', n.title, 'body', n.body, 'status', n.status,
				'is_read', n.is_read, 'created_at', n.created_at)
			  FROM notifications.logs n
			 WHERE n.user_id = $8 AND $1::bigint > 0
			   -- The notification centre's rule: a row addressed to this user,
			   -- and only when its permission is still held. Matching the
			   -- organisation as well read every colleague's notifications.
			   AND (COALESCE(n.required_permission, '') = '' OR n.required_permission = ANY($9))
			   AND ($3::text = '' OR ($3 = 'read' AND n.is_read) OR ($3 = 'unread' AND NOT n.is_read))
			   AND ($2::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY n.created_at DESC LIMIT $6 OFFSET $7`, append(args, actor.UserID, heldPermissions(actor)), q.Limit, q.Offset, false)
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
	default:
		return r.readPharmacyProjectionExt(ctx, actor, q)
	}
}
