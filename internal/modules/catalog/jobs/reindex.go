package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
)

// ProductReindexWorker synchronizes the catalog.product_index read model asynchronously.
type ProductReindexWorker struct {
	river.WorkerDefaults[queue.ProductReindexArgs]
	db  *database.DB
	log *slog.Logger
}

// NewProductReindexWorker constructs the reindexing river worker.
func NewProductReindexWorker(db *database.DB, log *slog.Logger) *ProductReindexWorker {
	return &ProductReindexWorker{db: db, log: log}
}

// Work handles both single-product idempotent upserts and full catalogue reindexing sweeps.
//
// Both paths write parent rows and variant rows. See reindex_sql.go for why the
// variant half exists and what it fixes.
func (w *ProductReindexWorker) Work(ctx context.Context, job *river.Job[queue.ProductReindexArgs]) error {
	pID := job.Args.ProductID
	w.log.InfoContext(ctx, "processing product reindex job", "product_id", pID)

	return w.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if pID == 0 {
			return w.sweep(txCtx, tx)
		}
		return w.upsertOne(txCtx, tx, pID)
	})
}

// sweep rebuilds the whole read model.
func (w *ProductReindexWorker) sweep(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `TRUNCATE TABLE catalog.product_index;`); err != nil {
		return fmt.Errorf("truncate product_index: %w", err)
	}

	parents, err := tx.Exec(ctx, sweepParents)
	if err != nil {
		return fmt.Errorf("reindex parents: %w", err)
	}

	variants, err := tx.Exec(ctx, sweepVariants)
	if err != nil {
		return fmt.Errorf("reindex variants: %w", err)
	}

	w.log.InfoContext(ctx, "product index rebuilt",
		"parents", parents.RowsAffected(),
		"variants", variants.RowsAffected())

	// A rebuild that produced no variant rows means either an empty catalogue or
	// a broken join, and the second is indistinguishable from the first at the
	// call site. Saying so here is what turns a silent wrong answer into a
	// visible one — the previous version of this job wrote NULL variant ids for
	// every row and nothing ever reported it.
	if variants.RowsAffected() == 0 && parents.RowsAffected() > 0 {
		w.log.WarnContext(ctx, "product index has parents but no variants; vendor offers will be invisible")
	}

	return nil
}

// upsertOne refreshes one product and all of its variants.
func (w *ProductReindexWorker) upsertOne(ctx context.Context, tx pgx.Tx, productID int64) error {
	if _, err := tx.Exec(ctx, upsertParent, productID); err != nil {
		return fmt.Errorf("upsert parent %d: %w", productID, err)
	}
	if _, err := tx.Exec(ctx, upsertVariants, productID); err != nil {
		return fmt.Errorf("upsert variants of %d: %w", productID, err)
	}
	if _, err := tx.Exec(ctx, pruneVariants, productID); err != nil {
		return fmt.Errorf("prune stale variants of %d: %w", productID, err)
	}
	return nil
}
