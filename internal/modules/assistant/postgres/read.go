package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The assistant's read model.
//
// It reads other modules' schemas directly rather than importing their
// services, because modules must not import each other (ADR 0002) and because
// what the assistant needs is not what those services expose: flat projections
// shaped for a prompt, not domain objects with relations the model would then
// carry around for the rest of the conversation.
//
// Two rules hold everywhere in this file and its siblings:
//
//   - Every read runs inside InReadTx WITHOUT AsSystem, except where a comment
//     says otherwise and explains why. That is what puts row-level security in
//     the path: `SET LOCAL app.current_org_id` is issued from the request
//     context, and the policies compare against it. A query that forgot its own
//     scoping returns zero rows rather than somebody else's.
//
//   - Every value that reaches SQL is a bound parameter. Nothing is
//     concatenated, including sort direction and search terms.

var _ assistant.Reader = (*Repository)(nil)

// ErrNoOrganization is returned when a tool needs a tenant and the caller has
// none. It is a refusal, not a failure.
var ErrNoOrganization = fmt.Errorf("assistant: caller has no organization")

// scopeOf is the one place the assistant turns an actor into a query scope.
func scopeOf(actor authctx.Actor) (int64, error) {
	orgID := actor.OrgID
	if orgID <= 0 {
		orgID = actor.OrganizationID
	}
	if orgID <= 0 {
		return 0, ErrNoOrganization
	}
	return orgID, nil
}

// amount converts a NUMERIC(12,2) read as a string into exact money.
//
// The database type is decimal and money.Amount is integer minor units. Going
// through the string form is deliberate: a float64 hop here would lose a
// fraction of a piastre per row and produce totals that do not reconcile with
// the invoices they came from.
func amount(s string) money.Amount {
	if strings.TrimSpace(s) == "" {
		return money.FromMinor(0)
	}
	a, err := money.Parse(s)
	if err != nil {
		return money.FromMinor(0)
	}
	return a
}

// localized picks the Arabic name from a bilingual JSONB column, falling back
// to English and then to a placeholder, so a row with a missing translation
// still reads as something rather than as an empty cell.
const localizedName = `COALESCE(NULLIF(%[1]s->>'ar',''), NULLIF(%[1]s->>'en',''), NULLIF(%[1]s#>>'{}',''), '')`

func nameExpr(col string) string {
	return fmt.Sprintf(localizedName, col)
}

// dateFilter appends an inclusive period filter, returning the new argument
// list and the SQL fragment. Bound parameters throughout.
func dateFilter(col string, r assistant.DateRange, args []any) (string, []any) {
	var b strings.Builder
	if !r.From.IsZero() {
		args = append(args, r.From)
		fmt.Fprintf(&b, " AND %s >= $%d", col, len(args))
	}
	if !r.To.IsZero() {
		args = append(args, r.To)
		fmt.Fprintf(&b, " AND %s <= $%d", col, len(args))
	}
	return b.String(), args
}

func rangeBounds(r assistant.DateRange) (*time.Time, *time.Time) {
	var from, to *time.Time
	if !r.From.IsZero() {
		f := r.From
		from = &f
	}
	if !r.To.IsZero() {
		t := r.To
		to = &t
	}
	return from, to
}

// pageOf trims an over-fetched slice down to the requested size and reports
// whether more rows exist.
//
// Every listing asks the database for limit+1 rows. That answers "is there a
// next page" without a second COUNT query, which on these tables is the
// expensive half of the request.
func pageOf[T any](rows []T, limit, offset int) assistant.Page[T] {
	p := assistant.Page[T]{}
	if len(rows) > limit {
		p.HasMore = true
		p.NextOffset = offset + limit
		rows = rows[:limit]
	}
	p.Rows = rows
	return p
}

// ---------------------------------------------------------------------------
// Shared reads
// ---------------------------------------------------------------------------

// Branches lists the caller's own branches.
func (r *Repository) Branches(ctx context.Context, actor authctx.Actor) ([]assistant.BranchRow, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	var out []assistant.BranchRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT b.id, `+nameExpr("b.name")+`, COALESCE(b.phone,''),
			       COALESCE(b.address,''), b.is_main, b.status
			  FROM org.branches b
			 WHERE b.organization_id = $1 AND b.deleted_at IS NULL
			 ORDER BY b.is_main DESC, b.id ASC
			 LIMIT 100;
		`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.BranchRow
			if err := rows.Scan(&row.ID, &row.Name, &row.Phone, &row.City,
				&row.IsMain, &row.Status); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: branches: %w", err)
	}
	return out, nil
}

// Wallet returns the organisation's balance and its recent movements.
//
// The balance is the running balance_after of the newest transaction rather
// than a sum, because that is the column the ledger maintains and the two must
// not be able to disagree.
func (r *Repository) Wallet(ctx context.Context, actor authctx.Actor) (*assistant.WalletSummary, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	summary := &assistant.WalletSummary{Currency: "EGP", Balance: money.FromMinor(0)}
	found := false

	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var walletID int64
		var currency string
		err := tx.QueryRow(txCtx, `
			SELECT w.id, w.currency
			  FROM billing.wallets w
			 WHERE w.organization_id = $1 OR w.user_id = $2
			 ORDER BY (w.organization_id = $1) DESC, w.id ASC
			 LIMIT 1;
		`, orgID, actor.UserID).Scan(&walletID, &currency)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		found = true
		summary.Currency = currency

		rows, err := tx.Query(txCtx, `
			SELECT t.type, t.amount::text, t.balance_after::text,
			       COALESCE(t.description,''), t.created_at
			  FROM billing.wallet_transactions t
			 WHERE t.wallet_id = $1
			 ORDER BY t.id DESC
			 LIMIT 10;
		`, walletID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tx0 assistant.WalletTxRow
			var amt, bal string
			if err := rows.Scan(&tx0.Type, &amt, &bal, &tx0.Description, &tx0.At); err != nil {
				return err
			}
			tx0.Amount = amount(amt)
			tx0.BalanceAter = amount(bal)
			summary.Recent = append(summary.Recent, tx0)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(summary.Recent) > 0 {
			summary.Balance = summary.Recent[0].BalanceAter
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: wallet: %w", err)
	}
	if !found {
		return nil, nil
	}
	return summary, nil
}

// Subscription returns the organisation's current plan.
func (r *Repository) Subscription(ctx context.Context, actor authctx.Actor) (*assistant.SubscriptionSummary, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	var out *assistant.SubscriptionSummary
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var s assistant.SubscriptionSummary
		var priceMonth string
		err := tx.QueryRow(txCtx, `
			SELECT `+nameExpr("p.name")+`, s.status, s.starts_at, s.expires_at,
			       p.price_month::text
			  FROM billing.subscriptions s
			  JOIN billing.plans p ON p.id = s.plan_id
			 WHERE s.organization_id = $1
			 ORDER BY (s.status = 'active') DESC, s.starts_at DESC, s.id DESC
			 LIMIT 1;
		`, orgID).Scan(&s.PlanName, &s.Status, &s.StartsAt, &s.ExpiresAt, &priceMonth)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		s.PriceMonth = amount(priceMonth).String()
		remaining := int(time.Until(s.ExpiresAt).Hours() / 24)
		if remaining < 0 {
			remaining = 0
		}
		s.DaysRemaining = remaining
		out = &s
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: subscription: %w", err)
	}
	return out, nil
}
