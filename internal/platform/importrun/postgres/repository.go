// Package postgres implements importrun.Repository using pgx against
// the platform.import_runs and platform.import_run_rows tables.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
)

// Repository implements importrun.Repository.
type Repository struct {
	db *database.DB
}

// New creates a Repository backed by the shared DB pool.
func New(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateRun inserts a new run.
func (r *Repository) CreateRun(ctx context.Context, run *importrun.Run) error {
	if run.Payload == nil {
		run.Payload = json.RawMessage(`{}`)
	}
	if run.Result == nil {
		run.Result = json.RawMessage(`{}`)
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO platform.import_runs (
				organization_id, user_id, kind, audience, filename,
				state, phase, percent, total_rows, processed_rows,
				payload, result, error_message
			) VALUES ($1,$2,$3,$4,$5, $6,$7,$8,$9,$10, $11,$12,$13)
			RETURNING id, public_id, created_at, updated_at
		`,
			run.OrganizationID, run.UserID, run.Kind, run.Audience, run.Filename,
			run.State, run.Phase, run.Percent, run.TotalRows, run.ProcessedRows,
			run.Payload, run.Result, run.ErrorMessage,
		).Scan(&run.ID, &run.PublicID, &run.CreatedAt, &run.UpdatedAt)
	})
}

// GetRunByPublicID fetches a run scoped to an org.
func (r *Repository) GetRunByPublicID(ctx context.Context, publicID string, orgID int64) (*importrun.Run, error) {
	run := &importrun.Run{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanRun(tx.QueryRow(txCtx, runSelectSQL+" WHERE r.public_id = $1 AND r.organization_id = $2", publicID, orgID), run)
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// GetRunByPublicIDSystem fetches a run without tenant scoping.
func (r *Repository) GetRunByPublicIDSystem(ctx context.Context, publicID string) (*importrun.Run, error) {
	run := &importrun.Run{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanRun(tx.QueryRow(txCtx, runSelectSQL+" WHERE r.public_id = $1", publicID), run)
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// GetRunByID fetches a run by internal ID.
func (r *Repository) GetRunByID(ctx context.Context, id int64) (*importrun.Run, error) {
	run := &importrun.Run{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanRun(tx.QueryRow(txCtx, runSelectSQL+" WHERE r.id = $1", id), run)
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// UpdateProgress sets progress fields on a run.
func (r *Repository) UpdateProgress(ctx context.Context, id int64, phase string, percent int, processed int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs
			SET phase = $2, percent = $3, processed_rows = $4
			WHERE id = $1
		`, id, phase, percent, processed)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("importrun: run %d not found", id)
		}
		return nil
	})
}

// TransitionState moves a run to a new state.
func (r *Repository) TransitionState(ctx context.Context, id int64, newState string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var setClause string
		switch newState {
		case importrun.StateProcessing:
			setClause = "state = $2, started_at = now()"
		case importrun.StateCommitted, importrun.StateFailed, importrun.StateCancelled:
			setClause = "state = $2, finished_at = now()"
		default:
			setClause = "state = $2"
		}
		tag, err := tx.Exec(txCtx, fmt.Sprintf(`
			UPDATE platform.import_runs SET %s WHERE id = $1
		`, setClause), id, newState)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("importrun: run %d not found", id)
		}
		return nil
	})
}

// FailRun transitions to 'failed' with an error message.
func (r *Repository) FailRun(ctx context.Context, id int64, errMsg string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs
			SET state = 'failed', error_message = $2, finished_at = now(), percent = 0
			WHERE id = $1
		`, id, errMsg)
		return err
	})
}

// SetResult stores summary counters.
func (r *Repository) SetResult(ctx context.Context, id int64, result json.RawMessage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs SET result = $2 WHERE id = $1
		`, id, result)
		return err
	})
}

// SetRiverJobID records the River job ID.
func (r *Repository) SetRiverJobID(ctx context.Context, id int64, jobID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs SET river_job_id = $2 WHERE id = $1
		`, id, jobID)
		return err
	})
}

// UpdatePayload updates the JSONB payload on a run.
func (r *Repository) UpdatePayload(ctx context.Context, id int64, payload json.RawMessage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs SET payload = $2, updated_at = now() WHERE id = $1
		`, id, payload)
		return err
	})
}

// GetActiveRunForUser finds the most recent unfinished run for a user and import kind.
func (r *Repository) GetActiveRunForUser(ctx context.Context, userID int64, kind string) (*importrun.Run, error) {
	run := &importrun.Run{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanRun(tx.QueryRow(txCtx, runSelectSQL+`
			WHERE r.user_id = $1 AND r.kind = $2
			  AND r.state NOT IN ('committed', 'failed', 'cancelled')
			  AND r.phase != 'done'
			ORDER BY r.id DESC LIMIT 1
		`, userID, kind), run)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return run, nil
}

// ListRunsByOrg returns recent runs for an org, newest first.
func (r *Repository) ListRunsByOrg(ctx context.Context, orgID int64, kind string, limit, offset int) ([]*importrun.Run, int, error) {
	var runs []*importrun.Run
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := "WHERE r.organization_id = $1"
		args := []any{orgID}
		argN := 2

		if kind != "" {
			where += fmt.Sprintf(" AND r.kind = $%d", argN)
			args = append(args, kind)
			argN++
		}

		// Count.
		countArgs := make([]any, len(args))
		copy(countArgs, args)
		if err := tx.QueryRow(txCtx,
			"SELECT COUNT(*) FROM platform.import_runs r "+where,
			countArgs...,
		).Scan(&total); err != nil {
			return err
		}

		// Rows.
		args = append(args, limit, offset)
		query := runSelectSQL + " " + where +
			fmt.Sprintf(" ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			run := &importrun.Run{}
			if err := scanRunRows(rows, run); err != nil {
				return err
			}
			runs = append(runs, run)
		}
		return rows.Err()
	})
	return runs, total, err
}
