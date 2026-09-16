package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// AppendEvent records progress.
//
// Events are stored rather than logged because they serve two audiences: the
// live progress bar, and the buyer asking six weeks later why a line was
// excluded. A log line answers neither once the process has restarted.
func (r *Repository) AppendEvent(ctx context.Context, e *smartorder.Event) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		msg, err := json.Marshal(e.Message)
		if err != nil {
			return err
		}
		if e.Level == "" {
			e.Level = "info"
		}
		return tx.QueryRow(txCtx, `
			INSERT INTO smartorder.run_events (
				run_id, organization_id, stage, processed, total, message, level
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING id, created_at;`,
			e.RunID, e.OrganizationID, string(e.Stage), e.Processed, e.Total, msg, e.Level,
		).Scan(&e.ID, &e.CreatedAt)
	})
}

// ListEvents returns events after a cursor, which is what lets a reconnecting
// progress page resume without replaying everything.
func (r *Repository) ListEvents(ctx context.Context, runID int64, afterID int64) ([]*smartorder.Event, error) {
	var out []*smartorder.Event
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, run_id, organization_id, stage, processed, total, message, level, created_at
			FROM smartorder.run_events
			WHERE run_id = $1 AND id > $2
			ORDER BY id LIMIT 500;`, runID, afterID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e smartorder.Event
			var stage string
			var msg []byte
			if err := rows.Scan(&e.ID, &e.RunID, &e.OrganizationID, &stage,
				&e.Processed, &e.Total, &msg, &e.Level, &e.CreatedAt); err != nil {
				return err
			}
			e.Stage = smartorder.Stage(stage)
			if len(msg) > 0 {
				_ = json.Unmarshal(msg, &e.Message)
			}
			out = append(out, &e)
		}
		return rows.Err()
	})
	return out, err
}
