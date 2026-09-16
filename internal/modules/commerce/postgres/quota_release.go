package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The supplier resetting one branch's consumption.
//
// Releasing writes an instant, not a credit. Everything the branch bought up to
// that moment stops counting and it starts again from zero against the same
// cap; every other branch is untouched, because every branch has its own
// allowance.
//
// The row is upserted rather than appended: a pairing has one current cut-off
// and there is no sensible reading of two. release_count keeps the fact that it
// has happened more than once, which is what a supplier wants to see before
// doing it a third time.

// ReleaseBranchQuota resets one buying branch's consumption of one variant.
//
// The variant's ownership is proved in SQL rather than by the caller: the
// INSERT only happens if the variant really belongs to this supplier and really
// carries a quota. A supplier cannot release a branch's consumption of somebody
// else's item, and cannot leave a stray release row on an item that has no
// quota to release.
func (r *Repository) ReleaseBranchQuota(
	ctx context.Context, vendorOrgID, variantID, branchID, actorUserID int64, note string,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Serialise against a checkout for the same pairing. Without it a
		// release landing mid-checkout could let an order through that the
		// supplier believes it authorised and a second that it did not.
		if err := lockQuotaPairing(txCtx, tx, variantID, branchID); err != nil {
			return err
		}

		var actor *int64
		if actorUserID > 0 {
			actor = &actorUserID
		}

		tag, err := tx.Exec(txCtx, `
			INSERT INTO commerce.variant_branch_quota_releases
			    (organization_id, product_variant_id, branch_id, released_at, released_by, note)
			SELECT $1, v.id, b.id, now(), $4, $5
			FROM catalog.product_variants v
			JOIN org.branches b ON b.id = $3 AND b.deleted_at IS NULL
			WHERE v.id = $2
			  AND v.organization_id = $1
			  AND v.deleted_at IS NULL
			ON CONFLICT (product_variant_id, branch_id) DO UPDATE
			SET released_at   = now(),
			    released_by   = EXCLUDED.released_by,
			    note          = EXCLUDED.note,
			    release_count = commerce.variant_branch_quota_releases.release_count + 1,
			    updated_at    = now();
		`, vendorOrgID, variantID, branchID, actor, note)
		if err != nil {
			return fmt.Errorf("commerce postgres: release branch quota: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Either the variant is not this supplier's, or the branch is gone.
			// Both are "there is nothing here to release" from the caller's
			// side, and neither should say which.
			return apperr.NotFound("variant_branch_quota")
		}
		return nil
	})
}

// UndoBranchQuotaRelease removes a reset, putting the branch's consumption back.
//
// Deleting the row rather than moving released_at backwards: the cut-off is the
// row's whole meaning, and "-infinity" and "no row" would then both mean "never
// released" while only one of them showed a release_count on the screen.
func (r *Repository) UndoBranchQuotaRelease(
	ctx context.Context, vendorOrgID, variantID, branchID int64,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := lockQuotaPairing(txCtx, tx, variantID, branchID); err != nil {
			return err
		}
		tag, err := tx.Exec(txCtx, `
			DELETE FROM commerce.variant_branch_quota_releases
			WHERE organization_id = $1 AND product_variant_id = $2 AND branch_id = $3;
		`, vendorOrgID, variantID, branchID)
		if err != nil {
			return fmt.Errorf("commerce postgres: undo quota release: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("variant_branch_quota_release")
		}
		return nil
	})
}
