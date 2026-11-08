package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// PurgeExpiredAsSystem sweeps the ERP's own tables and the ERP's catalog
// match_runs by their erp.sync consumer_ref prefix (plan.md E3: a named
// exception — retention is platform-wide bookkeeping, the cut-offs are the
// Retention* constants, and every statement is bounded by BatchRows so a
// backlog drains over runs instead of in one giant lock-holding delete).
//
// order_imports and erp.idempotency_records are NEVER purged from here: they
// are the ledger the ERP integrations reconcile against.
func (r *Repository) PurgeExpiredAsSystem(ctx context.Context, cut erp.Cutoffs) error {
	batch := cut.BatchRows
	if batch <= 0 {
		batch = 10000
	}
	sweeps := []struct {
		table    string
		timeCol  string
		deadline interface{}
		extra    string
	}{
		{table: "erp.api_calls", timeCol: "created_at", deadline: cut.APICalls},
		{table: "erp.sync_items", timeCol: "created_at", deadline: cut.SyncItems},
		{table: "erp.sync_batches", timeCol: "accepted_at", deadline: cut.SyncBatches},
		{table: "erp.review_items", timeCol: "updated_at", deadline: cut.Review,
			extra: ` AND state IN ('resolved','dismissed')`},
		{table: "catalog.match_runs", timeCol: "created_at", deadline: cut.MatchRuns,
			extra: ` AND consumer_ref LIKE 'erp.sync:%'`},
	}
	for _, sweep := range sweeps {
		for {
			var done int64
			err := r.db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, fmt.Sprintf(
					`DELETE FROM %s WHERE id IN (
						SELECT id FROM %s WHERE %s < $1%s LIMIT $2
					)`, sweep.table, sweep.table, sweep.timeCol, sweep.extra),
					sweep.deadline, batch)
				if err != nil {
					return err
				}
				done = tag.RowsAffected()
				return nil
			})
			if err != nil {
				return fmt.Errorf("erp: purge %s: %w", sweep.table, err)
			}
			if done < int64(batch) {
				break
			}
		}
	}
	return nil
}
