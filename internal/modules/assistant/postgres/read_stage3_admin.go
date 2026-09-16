package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readAdminProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	if err := requireStaff(actor); err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := adminProjectionArgs(q)
	switch q.Kind {
	case assistant.ProjectionApprovals:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object('name', `+nameExpr("o.name")+`,
				'type', o.type, 'status', o.status, 'created_at', o.created_at,
				'documents', (SELECT COUNT(*) FROM platform_admin.documents d WHERE d.organization_id=o.id AND d.status='pending'))
			  FROM org.organizations o WHERE o.status='pending' AND o.deleted_at IS NULL
			   AND ($1='' OR `+nameExpr("o.name")+` ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY o.created_at ASC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionDeletionRequests:
		return r.readProjectionRows(ctx, actor, `
			SELECT dr.id, jsonb_build_object('organization', COALESCE(`+nameExpr("o.name")+`,''),
				'requester', COALESCE(`+nameExpr("u.name")+`,u.email,''), 'status', dr.status,
				'reason', COALESCE(dr.reason,''), 'created_at', dr.created_at)
			  FROM org.organization_deletion_requests dr
			  LEFT JOIN org.organizations o ON o.id=dr.organization_id
			  LEFT JOIN identity.users u ON u.id=dr.requested_by
			 WHERE ($2='' OR dr.status=$2)
			   AND ($1::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY dr.created_at DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionFinance:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'wallet_balance', COALESCE((SELECT SUM(wt.balance_after) FROM billing.wallet_transactions wt),0)::text,
				'invoices', (SELECT COUNT(*) FROM billing.invoices i WHERE ($3::timestamptz IS NULL OR i.created_at >= $3) AND ($4::timestamptz IS NULL OR i.created_at <= $4)),
				'payments', COALESCE((SELECT SUM(p.amount) FROM billing.payments p WHERE p.status IN ('paid','completed') AND ($3::timestamptz IS NULL OR p.created_at >= $3) AND ($4::timestamptz IS NULL OR p.created_at <= $4)),0)::text,
				'pending_withdrawals', (SELECT COUNT(*) FROM billing.wallet_withdrawals w WHERE w.status IN ('pending','requested')),
				'orders', (SELECT COUNT(*) FROM commerce.orders o WHERE ($3::timestamptz IS NULL OR o.created_at >= $3) AND ($4::timestamptz IS NULL OR o.created_at <= $4)))
			 WHERE ($1::text = '' OR TRUE) AND ($2::text = '' OR TRUE)`, args[:4], 1, 0, true)
	case assistant.ProjectionVisitors:
		return r.readProjectionRows(ctx, actor, `
			 SELECT ROW_NUMBER() OVER (ORDER BY v.city)::bigint, jsonb_build_object(
				'city', COALESCE(v.city,''), 'visits', COUNT(*), 'unique_visitors', COUNT(DISTINCT v.visitor_key))
			  FROM platform_admin.visitors v WHERE ($1::text = '' OR TRUE) AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR v.visited_at >= $3)
			   AND ($4::timestamptz IS NULL OR v.visited_at <= $4)
			 GROUP BY v.city ORDER BY COUNT(*) DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionHealth:
		return r.readProjectionRows(ctx, actor, healthSQL, nil, 1, 0, true)
	case assistant.ProjectionMatchDecisions:
		return r.readProjectionRows(ctx, actor, `
			 SELECT md.id, jsonb_build_object('key', md.decision_key, 'name', md.norm_name,
				'confidence', md.confidence::text, 'scope', md.scope, 'hits', md.hit_count,
				'created_at', md.created_at, 'last_used_at', md.last_used_at)
			  FROM catalog.match_decisions md WHERE ($1='' OR md.decision_key ILIKE '%' || $1 || '%' OR md.norm_name ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR md.created_at >= $3) AND ($4::timestamptz IS NULL OR md.created_at <= $4)
			 ORDER BY md.created_at DESC, md.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionInstitutionalGraph:
		return r.readProjectionRows(ctx, actor, `
			SELECT c.id, jsonb_build_object('from_work', `+nameExpr("fw.title")+`,
				'to_work', `+nameExpr("tw.title")+`, 'created_at', c.created_at,
				'buyers', COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id=c.from_institutional_work_id),0),
				'suppliers', COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id=c.to_institutional_work_id),0))
			  FROM org.institutional_work_connections c
			  JOIN org.institutional_works fw ON fw.id=c.from_institutional_work_id
			  JOIN org.institutional_works tw ON tw.id=c.to_institutional_work_id
			 WHERE ($1='' OR `+nameExpr("fw.title")+` ILIKE '%' || $1 || '%' OR `+nameExpr("tw.title")+` ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY c.created_at DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionSecurityEvents:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'failed_logins_24h', (SELECT COUNT(*) FROM platform.audit_log WHERE action = 'user.login.failed' AND created_at >= now() - interval '24 hours'),
				'denied_tool_calls_24h', (SELECT COUNT(*) FROM assistant.tool_audit WHERE decision <> 'allowed' AND created_at >= now() - interval '24 hours'),
				'active_sessions_count', (SELECT COUNT(*) FROM identity.user_security WHERE last_login_at >= now() - interval '24 hours'),
				'recent_audit_alerts', COALESCE((
					SELECT jsonb_agg(jsonb_build_object(
						'action', a.action,
						'actor', COALESCE(`+nameExpr("u.name")+`, u.email, 'unknown'),
						'entity_type', a.entity_type,
						'at', a.created_at))
					FROM (SELECT * FROM platform.audit_log
					       WHERE created_at >= now() - interval '24 hours'
					       ORDER BY id DESC LIMIT 5) a
					LEFT JOIN identity.users u ON u.id = a.actor_user_id), '[]'::jsonb))`, nil, 1, 0, true)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported admin projection %q", q.Kind)
	}
}

// healthSQL counts jobs by River's own states. The queue table it used to read
// holds one row per queue name, not jobs, so "queued" and "failed" were counts
// of queue definitions.
const healthSQL = `
	SELECT 1::bigint, jsonb_build_object(
		'errors_last_day', (SELECT COUNT(*) FROM platform_admin.error_logs WHERE created_at >= now()-interval '1 day'),
		'queued_jobs', (SELECT COUNT(*) FROM river_job WHERE state IN ('available','scheduled','retryable')),
		'running_jobs', (SELECT COUNT(*) FROM river_job WHERE state = 'running'),
		'failed_jobs_last_day', (SELECT COUNT(*) FROM river_job WHERE state = 'discarded' AND finalized_at >= now()-interval '1 day'),
		'checked_at', now())`
