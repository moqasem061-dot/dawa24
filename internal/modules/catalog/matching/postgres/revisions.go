package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// CurrentRevision returns the current monotonic revision number for a catalog scope.
func (r *Repository) CurrentRevision(ctx context.Context, scope string) (int64, error) {
	if scope == "" {
		scope = "master"
	}
	var rev int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			SELECT revision
			FROM catalog.catalog_revisions
			WHERE scope = $1
		`, scope).Scan(&rev)
		if errors.Is(err, pgx.ErrNoRows) {
			rev = 1
			return nil
		}
		return err
	})
	if err != nil {
		return 1, err
	}
	return rev, nil
}
