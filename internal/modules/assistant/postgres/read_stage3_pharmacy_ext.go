package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readPharmacyProjectionExt(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	switch q.Kind {
	case assistant.ProjectionFinancialObligations:
		// What the company owes as a buyer. Invoices are the ones issued TO it
		// (customer_org_id) and still open; the statuses are billing.invoices'
		// own. Orders in progress are its purchases not yet delivered or ended.
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'unpaid_invoices_total', COALESCE((SELECT SUM(i.total_amount) FROM billing.invoices i
					WHERE i.customer_org_id = $1 AND i.status IN ('issued','partially_paid','overdue')), 0)::text,
				'unpaid_invoices_count', (SELECT COUNT(*) FROM billing.invoices i
					WHERE i.customer_org_id = $1 AND i.status IN ('issued','partially_paid','overdue')),
				'overdue_invoices_count', (SELECT COUNT(*) FROM billing.invoices i
					WHERE i.customer_org_id = $1 AND (i.status = 'overdue' OR (i.status IN ('issued','partially_paid') AND i.due_date < current_date))),
				'in_progress_orders_total', COALESCE((SELECT SUM(o.total_amount) FROM commerce.orders o
					WHERE o.organization_id = $1 AND o.deleted_at IS NULL
					  AND o.status IN ('pending','processing','confirmed','on_hold','shipped','in_transit','out_for_delivery')), 0)::text,
				'in_progress_orders_count', (SELECT COUNT(*) FROM commerce.orders o
					WHERE o.organization_id = $1 AND o.deleted_at IS NULL
					  AND o.status IN ('pending','processing','confirmed','on_hold','shipped','in_transit','out_for_delivery')),
				'currency', 'EGP')`, []any{orgID}, 1, 0, false)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported pharmacy projection %q", q.Kind)
	}
}
