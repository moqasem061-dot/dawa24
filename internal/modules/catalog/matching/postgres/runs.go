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

// GetRun retrieves a match run by ID under tenant isolation.
func (r *Repository) GetRun(ctx context.Context, runID int64) (*matching.MatchRun, error) {
	var run matching.MatchRun
	var timingsJSON, countsJSON, aiJSON []byte

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, workflow, entry_point, consumer_ref,
			       source_revision, policy_version, catalog_revision, normalizer_version,
			       state, stage_timings, counts, ai, heartbeat_at, created_by, created_at, finished_at
			FROM catalog.match_runs
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, runID).Scan(
			&run.ID, &run.OrganizationID, &run.Workflow, &run.EntryPoint, &run.ConsumerRef,
			&run.SourceRevision, &run.PolicyVersion, &run.CatalogRevision, &run.NormalizerVersion,
			&run.State, &timingsJSON, &countsJSON, &aiJSON, &run.HeartbeatAt, &run.CreatedBy, &run.CreatedAt, &run.FinishedAt,
		)
	})
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(timingsJSON, &run.StageTimings)
	_ = json.Unmarshal(countsJSON, &run.Counts)
	_ = json.Unmarshal(aiJSON, &run.AI)

	return &run, nil
}

// ListRunRows fetches rows associated with a match run for diagnostic inspection.
func (r *Repository) ListRunRows(ctx context.Context, runID int64, workflow string, limit int) ([]matching.DiagnosticRowData, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []matching.DiagnosticRowData

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		switch workflow {
		case "vendor":
			query = `SELECT row_number, COALESCE(display_name, ''), match_decision, match_state, match_basis, match_revision, matched_product_id
			         FROM ingest.catalog_import_rows WHERE match_run_id = $1 ORDER BY row_number LIMIT $2;`
		case "compare":
			query = `SELECT row_number, COALESCE(raw_name, ''), match_decision, match_state, match_basis, match_revision, matched_product_id
			         FROM compare.file_rows WHERE match_run_id = $1 ORDER BY row_number LIMIT $2;`
		case "smartorder":
			query = `SELECT id, COALESCE(raw_name, ''), match_decision, match_state, match_basis, match_revision, matched_product_id
			         FROM smartorder.run_lines WHERE match_run_id = $1 ORDER BY id LIMIT $2;`
		default:
			query = `SELECT id, COALESCE(raw_name, ''), match_decision, match_state, match_basis, match_revision, matched_product_id
			         FROM catalog.import_staging_rows WHERE match_run_id = $1 ORDER BY id LIMIT $2;`
		}

		rows, qErr := tx.Query(txCtx, query, runID, limit)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()

		idx := 0
		for rows.Next() {
			var rowNum int
			var name string
			var state, basis *string
			var rev int
			var pID *int64
			var decJSON []byte

			if scanErr := rows.Scan(&rowNum, &name, &decJSON, &state, &basis, &rev, &pID); scanErr == nil {
				var dec matchflow.RowDecision
				if len(decJSON) > 0 {
					_ = json.Unmarshal(decJSON, &dec)
				}
				sVal, bVal := "", ""
				if state != nil {
					sVal = *state
				}
				if basis != nil {
					bVal = *basis
				}
				out = append(out, matching.DiagnosticRowData{
					Index:       idx,
					DisplayName: name,
					State:       sVal,
					Basis:       bVal,
					Revision:    rev,
					ProductID:   pID,
					Decision:    dec,
				})
				idx++
			}
		}
		return nil
	})

	return out, err
}
