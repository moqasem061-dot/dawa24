package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// DatasetRole is the database role every dataset statement runs as. See
// db/migrations/214_assistant_dataset_reader.up.sql for what it may read.
const DatasetRole = "dawa24_assistant_ro"

// RunDataset executes a compiled plan.
//
// Three things bound it, all set inside the transaction so they end with it:
// the transaction is read-only, the role can only SELECT granted columns, and
// the statement timeout stops a question that turned into a table scan from
// holding a pooled connection.
func (r *Repository) RunDataset(ctx context.Context, plan *datasets.Plan, timeout time.Duration) (*datasets.Result, error) {
	res := &datasets.Result{Columns: plan.Columns}
	n := len(plan.Columns)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, "SAVEPOINT assume_dataset_role"); err == nil {
			if _, err := tx.Exec(txCtx, "SET LOCAL ROLE "+DatasetRole); err != nil {
				_, _ = tx.Exec(txCtx, "ROLLBACK TO SAVEPOINT assume_dataset_role")
				return fmt.Errorf("dataset security: cannot assume role %q: %w", DatasetRole, err)
			}
			_, _ = tx.Exec(txCtx, "RELEASE SAVEPOINT assume_dataset_role")
		}
		if _, err := tx.Exec(txCtx, "SELECT set_config('statement_timeout', $1, true)",
			fmt.Sprintf("%dms", timeout.Milliseconds())); err != nil {
			return fmt.Errorf("set statement timeout: %w", err)
		}
		rows, err := tx.Query(txCtx, plan.SQL, plan.Args...)
		if err != nil {
			slog.Default().ErrorContext(txCtx, "dataset query failed",
				"dataset", plan.Dataset.Name, "sql", plan.SQL, "args", plan.Args, "error", err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			texts := make([]*string, n)
			dest := make([]any, 0, n+2)
			for i := range texts {
				dest = append(dest, &texts[i])
			}
			var id, total int64
			if plan.HasKey {
				dest = append(dest, &id)
			}
			dest = append(dest, &total)
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			values := make([]any, n)
			for i, col := range plan.Columns {
				values[i] = datasets.Value(col.Type, texts[i])
			}
			res.Rows = append(res.Rows, values)
			if plan.HasKey {
				res.Keys = append(res.Keys, id)
			}
			res.Total = int(total)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("assistant dataset %s: %w", plan.Dataset.Name, err)
	}
	if len(res.Rows) == 0 && plan.Offset > 0 {
		// A page past the end has no window row to read the total from.
		res.Total = plan.Offset
	}
	return res, nil
}
