package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// CreateRun inserts a new match_runs record under the active tenant transaction.
func (r *Repository) CreateRun(ctx context.Context, run *matching.MatchRun) error {
	timingsJSON, err := json.Marshal(run.StageTimings)
	if err != nil {
		timingsJSON = []byte("{}")
	}
	countsJSON, err := json.Marshal(run.Counts)
	if err != nil {
		countsJSON = []byte("{}")
	}
	aiJSON, err := json.Marshal(run.AI)
	if err != nil {
		aiJSON = []byte("{}")
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.match_runs (
				organization_id, workflow, entry_point, consumer_ref,
				source_revision, policy_version, catalog_revision, normalizer_version,
				state, stage_timings, counts, ai, heartbeat_at, created_by, created_at
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8,
				$9, $10, $11, $12, now(), $13, now()
			)
			RETURNING id, created_at, heartbeat_at;
		`
		createdBy := run.CreatedBy
		if createdBy == "" {
			createdBy = "system"
		}
		state := run.State
		if state == "" {
			state = "preparing"
		}

		return tx.QueryRow(txCtx, query,
			run.OrganizationID, run.Workflow, run.EntryPoint, run.ConsumerRef,
			run.SourceRevision, run.PolicyVersion, run.CatalogRevision, run.NormalizerVersion,
			state, timingsJSON, countsJSON, aiJSON, createdBy,
		).Scan(&run.ID, &run.CreatedAt, &run.HeartbeatAt)
	})
}

// UpdateCounts updates the counts JSON and heartbeat timestamp.
func (r *Repository) UpdateCounts(ctx context.Context, runID int64, counts matchflow.RunCounts) error {
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return err
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_runs
			SET counts = $1, heartbeat_at = now()
			WHERE id = $2
		`, countsJSON, runID)
		return err
	})
}

// SetState updates the state and optionally sets finished_at.
func (r *Repository) SetState(ctx context.Context, runID int64, state string, finished bool) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var finishedAt *time.Time
		if finished {
			now := time.Now().UTC()
			finishedAt = &now
		}

		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_runs
			SET state = $1,
			    finished_at = COALESCE($2, finished_at),
			    heartbeat_at = now()
			WHERE id = $3
		`, state, finishedAt, runID)
		return err
	})
}

// Heartbeat refreshes the heartbeat timestamp.
func (r *Repository) Heartbeat(ctx context.Context, runID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_runs
			SET heartbeat_at = now()
			WHERE id = $1
		`, runID)
		return err
	})
}
