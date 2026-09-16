package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The credit ledger.
//
// The counter and its history move in one transaction. Writing the entry from
// the service after the UPDATE returned would give a statement that omits every
// movement whose second write failed, which is exactly the class of silence
// this table exists to end.

// ConsumeSponsorshipCredits moves a purchase's balance and records why.
//
// The UPDATE keeps the guards the old increment had — the purchase must be
// active, unexpired, and must not go past credits_total — and adds one the old
// one lacked: a refund cannot drive credits_used below zero, which would have
// manufactured credits out of a double-refund.
func (r *Repository) ConsumeSponsorshipCredits(
	ctx context.Context, in promo.ConsumeCredits,
) (*promo.CreditEntry, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	var entry promo.CreditEntry
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		used := in.Credits
		if in.Refund {
			used = -in.Credits
		}

		const updateSQL = `
			UPDATE promo.sponsorship_purchases
			SET credits_used = credits_used + $2, updated_at = now()
			WHERE id = $1
			  AND credits_used + $2 <= credits_total
			  AND credits_used + $2 >= 0
			  AND ($3 OR (status = 'active' AND expires_at > now()))
			RETURNING organization_id, GREATEST(credits_total - credits_used, 0);`

		var orgID int64
		var balanceAfter int
		// A refund is allowed against an expired or cancelled purchase: the
		// credit was taken while it was live, and refusing to give it back
		// because the package has since lapsed would keep money the platform
		// did not earn.
		err := tx.QueryRow(txCtx, updateSQL, in.PurchaseID, used, in.Refund).
			Scan(&orgID, &balanceAfter)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.Conflict("sponsorship.insufficient_credits",
					i18n.TDefault("w4_mod.w4str_253_253"))
			}
			return fmt.Errorf("promo postgres: move sponsorship credits: %w", err)
		}

		const insertSQL = `
			INSERT INTO promo.sponsorship_credit_entries (
				organization_id, purchase_id, delta, balance_after,
				reason, entity_type, entity_id, actor_user_id, note
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at;`

		entry = promo.CreditEntry{
			OrganizationID: orgID,
			PurchaseID:     in.PurchaseID,
			Delta:          in.Delta(),
			BalanceAfter:   balanceAfter,
			Reason:         in.Reason,
			EntityType:     in.EntityType,
			EntityID:       in.EntityID,
			ActorUserID:    in.ActorUserID,
			Note:           in.Note,
		}
		return tx.QueryRow(txCtx, insertSQL,
			orgID, in.PurchaseID, entry.Delta, balanceAfter,
			string(in.Reason), in.EntityType, in.EntityID, in.ActorUserID, in.Note,
		).Scan(&entry.ID, &entry.PublicID, &entry.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

const creditEntryColumns = `
	id, public_id, organization_id, purchase_id, delta, balance_after,
	reason, entity_type, entity_id, actor_user_id, note, created_at`

func scanCreditEntry(rows pgx.Rows) (*promo.CreditEntry, error) {
	var e promo.CreditEntry
	err := rows.Scan(
		&e.ID, &e.PublicID, &e.OrganizationID, &e.PurchaseID, &e.Delta, &e.BalanceAfter,
		&e.Reason, &e.EntityType, &e.EntityID, &e.ActorUserID, &e.Note, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListCreditEntries returns one purchase's ledger, newest first.
func (r *Repository) ListCreditEntries(
	ctx context.Context, purchaseID int64, limit, offset int,
) ([]*promo.CreditEntry, int, error) {
	var (
		out   []*promo.CreditEntry
		total int
	)
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM promo.sponsorship_credit_entries WHERE purchase_id = $1;`,
			purchaseID,
		).Scan(&total); err != nil {
			return fmt.Errorf("promo postgres: count credit entries: %w", err)
		}

		// id breaks the tie: a batch writes several entries inside one
		// transaction and they share created_at to the microsecond, so ordering
		// by time alone would shuffle them between page loads.
		rows, err := tx.Query(txCtx, `
			SELECT`+creditEntryColumns+`
			FROM promo.sponsorship_credit_entries
			WHERE purchase_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;`, purchaseID, limit, offset)
		if err != nil {
			return fmt.Errorf("promo postgres: list credit entries: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			e, err := scanCreditEntry(rows)
			if err != nil {
				return fmt.Errorf("promo postgres: scan credit entry: %w", err)
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, total, err
}

// ListOrgCreditEntries returns a company's movements across every purchase, for
// the administrator's per-organization drill-down.
func (r *Repository) ListOrgCreditEntries(
	ctx context.Context, orgID int64, limit, offset int,
) ([]*promo.CreditEntry, int, error) {
	var (
		out   []*promo.CreditEntry
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM promo.sponsorship_credit_entries WHERE organization_id = $1;`,
			orgID,
		).Scan(&total); err != nil {
			return fmt.Errorf("promo postgres: count org credit entries: %w", err)
		}

		rows, err := tx.Query(txCtx, `
			SELECT`+creditEntryColumns+`
			FROM promo.sponsorship_credit_entries
			WHERE organization_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;`, orgID, limit, offset)
		if err != nil {
			return fmt.Errorf("promo postgres: list org credit entries: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			e, err := scanCreditEntry(rows)
			if err != nil {
				return fmt.Errorf("promo postgres: scan credit entry: %w", err)
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, total, err
}

// CreditTotals sums a purchase's movements in each direction.
//
// The statement header describes the whole purchase, so it cannot be summed
// from the page of rows in hand: a header that changed as the reader paged
// would be worse than no header at all.
func (r *Repository) CreditTotals(
	ctx context.Context, purchaseID int64,
) (consumed, refunded int, err error) {
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT COALESCE(-SUM(delta) FILTER (WHERE delta < 0), 0),
			       COALESCE( SUM(delta) FILTER (WHERE delta > 0), 0)
			FROM promo.sponsorship_credit_entries
			WHERE purchase_id = $1;`, purchaseID).Scan(&consumed, &refunded)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("promo postgres: credit totals: %w", err)
	}
	return consumed, refunded, nil
}
