package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Platform-wide reads.
//
// These are the only assistant queries that use AsSystem, and every one of them
// is guarded twice before it gets here: the tool declares rbac.ScopeAdmin, and
// Dispatch requires the caller to hold the specific admin permission the
// equivalent screen requires. Being staff is not sufficient for any of them.
//
// The shapes are deliberately coarse — registration records and counts. There
// is no admin tool that reads one tenant's order book, prices or documents. A
// staff member who needs that opens the screen, where the access is a page view
// somebody can audit rather than a sentence in a chat log.
//
// requireStaff is the third guard, and it exists because AsSystem is exactly
// the kind of call that must not be reachable by accident from a refactor.

func requireStaff(actor authctx.Actor) error {
	if !actor.IsStaff {
		return fmt.Errorf("assistant: platform read attempted by non-staff actor")
	}
	return nil
}

// PlatformOverview returns the operator's headline counts.
func (r *Repository) PlatformOverview(
	ctx context.Context, actor authctx.Actor, rng assistant.DateRange,
) (*assistant.PlatformSummary, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}

	orderWhere := ` WHERE o.deleted_at IS NULL`
	orderArgs := []any{}
	frag, orderArgs := dateFilter("o.created_at", rng, orderArgs)
	orderWhere += frag

	summary := &assistant.PlatformSummary{GMV: money.FromMinor(0), LifetimeGMV: money.FromMinor(0)}
	summary.From, summary.To = rangeBounds(rng)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// One pass over organisations rather than four COUNT queries: these
		// tables are small enough that the round trips cost more than the scan.
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*),
			       COUNT(*) FILTER (WHERE type IN ('customer','pharmacy','chain_pharmacy','individual')),
			       COUNT(*) FILTER (WHERE type IN ('vendor','supplier','company','agency')),
			       COUNT(*) FILTER (WHERE status IN ('pending', 'under_review'))
			  FROM org.organizations
			 WHERE deleted_at IS NULL;
		`).Scan(&summary.Organizations, &summary.Pharmacies,
			&summary.Vendors, &summary.PendingApproval); err != nil {
			return err
		}

		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*) FROM identity.users WHERE deleted_at IS NULL;
		`).Scan(&summary.Users); err != nil {
			return err
		}

		var gmv string
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*), COALESCE(SUM(o.total_amount),0)::text
			  FROM commerce.orders o`+orderWhere+`;
		`, orderArgs...).Scan(&summary.Orders, &gmv); err != nil {
			return err
		}
		summary.GMV = amount(gmv)

		var lifetimeGMV string
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*), COALESCE(SUM(o.total_amount),0)::text
			  FROM commerce.orders o WHERE o.deleted_at IS NULL;
		`).Scan(&summary.LifetimeOrders, &lifetimeGMV); err != nil {
			return err
		}
		summary.LifetimeGMV = amount(lifetimeGMV)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: platform overview: %w", err)
	}
	return summary, nil
}
