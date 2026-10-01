package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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

// CountActiveAIRuns counts how many runs are currently in 'enhancing' state for the organization.
func (r *Repository) CountActiveAIRuns(ctx context.Context, orgID int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT count(*) FROM catalog.match_runs WHERE organization_id = $1 AND state = 'enhancing';`
		return tx.QueryRow(txCtx, query, orgID).Scan(&count)
	})
	return count, err
}

// UpdateAIRunCounters updates the AI counters JSON and refreshes heartbeat.
func (r *Repository) UpdateAIRunCounters(ctx context.Context, runID int64, aiCounters map[string]any) error {
	aiJSON, err := json.Marshal(aiCounters)
	if err != nil {
		return err
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.match_runs
			SET ai = $1, heartbeat_at = now()
			WHERE id = $2;
		`
		_, execErr := tx.Exec(txCtx, query, aiJSON, runID)
		return execErr
	})
}

// RecordAIAttempt inserts a record into catalog.match_ai_attempts (T119, T125).
func (r *Repository) RecordAIAttempt(ctx context.Context, a *matching.AIAttemptRecord) error {
	var costStr *string
	if a.Cost != nil {
		s := a.Cost.String()
		costStr = &s
	}
	answersJSON := a.Answers
	if len(answersJSON) == 0 {
		answersJSON = []byte("[]")
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.match_ai_attempts (
				run_id, organization_id, attempt_no, kind,
				question_count, started_at, deadline_at, ended_at,
				outcome, input_tokens, output_tokens, cost,
				request_sha256, invalid_excerpt, answers, created_at
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8,
				$9, $10, $11, $12::numeric,
				$13, $14, $15, now()
			);
		`
		_, err := tx.Exec(txCtx, query,
			a.RunID, a.OrganizationID, a.AttemptNo, a.Kind,
			a.QuestionCount, a.StartedAt, a.DeadlineAt, a.EndedAt,
			a.Outcome, a.InputTokens, a.OutputTokens, costStr,
			a.RequestSHA256, a.InvalidExcerpt, answersJSON,
		)
		return err
	})
}

// ListEnhanceRows retrieves all rows associated with a match run as EnhanceRows for AI processing.
func (r *Repository) ListEnhanceRows(ctx context.Context, runID int64, workflow string) ([]matching.EnhanceRow, error) {
	var out []matching.EnhanceRow
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var query string
		switch workflow {
		case "vendor":
			query = `SELECT id, row_number, COALESCE(display_name, ''), match_decision, match_revision
			         FROM ingest.catalog_import_rows WHERE match_run_id = $1 ORDER BY row_number;`
		case "compare":
			query = `SELECT id, row_number, COALESCE(raw_name, ''), match_decision, match_revision
			         FROM compare.file_rows WHERE match_run_id = $1 ORDER BY row_number;`
		case "smartorder":
			query = `SELECT id, id, COALESCE(raw_name, ''), match_decision, match_revision
			         FROM smartorder.run_lines WHERE match_run_id = $1 ORDER BY id;`
		case "savings":
			query = `SELECT id, id, COALESCE(raw_name, ''), match_decision, match_revision
			         FROM platform.import_run_rows WHERE match_run_id = $1 ORDER BY id;`
		default:
			query = `SELECT id, id, COALESCE(raw_name, ''), match_decision, match_revision
			         FROM catalog.import_staging_rows WHERE match_run_id = $1 ORDER BY id;`
		}

		rows, qErr := tx.Query(txCtx, query, runID)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()

		for rows.Next() {
			var id int64
			var rowNum int
			var name string
			var decRaw []byte
			var rev int
			if err := rows.Scan(&id, &rowNum, &name, &decRaw, &rev); err != nil {
				return err
			}
			var dec matchflow.RowDecision
			if len(decRaw) > 0 {
				_ = json.Unmarshal(decRaw, &dec)
			}
			dec.Revision = rev
			out = append(out, matching.EnhanceRow{
				RowRef:   matching.RowRef{ID: id, RowNumber: rowNum},
				Row:      productmatch.Row{Name: name},
				Decision: dec,
			})
		}
		return nil
	})
	return out, err
}

// PurgeOldAIAttempts deletes match_ai_attempts rows older than maxAge across all tenants (US9 T129).
// Uses database.AsSystem because it is a platform maintenance cleanup job.
func (r *Repository) PurgeOldAIAttempts(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		maxAge = 30 * 24 * time.Hour
	}
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM catalog.match_ai_attempts WHERE created_at < now() - ($1 || ' seconds')::interval;`
		tag, execErr := tx.Exec(txCtx, query, fmt.Sprintf("%d", int(maxAge.Seconds())))
		if execErr != nil {
			return execErr
		}
		count = tag.RowsAffected()
		return nil
	})
	return count, err
}
