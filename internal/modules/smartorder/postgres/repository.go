// Package postgres implements smartorder.Repository against PostgreSQL.
//
// Every method runs inside db.InTx or db.InReadTx, which set the GUC that row
// level security reads. Nothing here reaches for db.Pool() directly.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository is the PostgreSQL implementation of smartorder.Repository.
type Repository struct {
	db *database.DB
}

// New constructs the repository.
func New(db *database.DB) *Repository { return &Repository{db: db} }

var _ smartorder.Repository = (*Repository)(nil)

const runColumns = `
	id, public_id, run_number, organization_id, user_id, branch_id, upload_id,
	COALESCE(original_filename, '') AS original_filename, status, current_step,
	total_rows, matched_rows, unmatched_rows, no_supplier_rows,
	coverage_blocked_rows, institutional_blocked_rows, below_min_qty_rows,
	COALESCE(quota_blocked_rows, 0) AS quota_blocked_rows,
	estimated_total, budget_exceeded, budget_overage,
	order_id, finalized_at,
	ai_enabled, ai_calls, ai_lines_reviewed, ai_lines_adjudicated, ai_lines_improved,
	ai_cache_hits, ai_cost_estimate, ai_ceiling_hit,
	deterministic_ms, total_ms, COALESCE(failure_reason, '') AS failure_reason,
	created_at, updated_at`

func scanRun(row pgx.Row) (*smartorder.Run, error) {
	var r smartorder.Run
	var status string
	var overage *money.Amount
	if err := row.Scan(
		&r.ID, &r.PublicID, &r.RunNumber, &r.OrganizationID, &r.UserID, &r.BranchID,
		&r.UploadID, &r.OriginalFile, &status, &r.CurrentStep,
		&r.Stats.TotalRows, &r.Stats.MatchedRows, &r.Stats.UnmatchedRows, &r.Stats.NoSupplierRows,
		&r.Stats.CoverageBlockedRows, &r.Stats.InstitutionalBlockedRows, &r.Stats.BelowMinQtyRows,
		&r.Stats.QuotaBlockedRows,
		&r.EstimatedTotal, &r.BudgetExceeded, &overage,
		&r.OrderID, &r.FinalizedAt,
		&r.AI.Enabled, &r.AI.Calls, &r.AI.LinesReviewed, &r.AI.LinesAdjudicated,
		&r.AI.LinesImproved, &r.AI.CacheHits, &r.AI.CostEstimate, &r.AI.CeilingHit,
		&r.DeterministicMS, &r.TotalMS, &r.FailureReason, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	r.Status = smartorder.RunStatus(status)
	r.BudgetOverage = overage
	return &r, nil
}

// CreateRun writes the run and its configuration snapshot in one transaction.
//
// The two must land together: a run whose configuration is missing cannot
// explain any decision it later makes.
func (r *Repository) CreateRun(ctx context.Context, run *smartorder.Run, cfg *smartorder.Config) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			INSERT INTO smartorder.runs (
				run_number, organization_id, user_id, branch_id, upload_id,
				original_filename, status, current_step, ai_enabled
			) VALUES (
				COALESCE(NULLIF($1,''), 'SO-' || to_char(now(),'YYYY') || '-' ||
					lpad(nextval('smartorder.run_number_seq')::text, 6, '0')),
				$2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, run_number, created_at, updated_at;`,
			run.RunNumber, run.OrganizationID, run.UserID, run.BranchID, run.UploadID,
			run.OriginalFile, string(run.Status), run.CurrentStep, run.AI.Enabled,
		).Scan(&run.ID, &run.PublicID, &run.RunNumber, &run.CreatedAt, &run.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert run: %w", err)
		}

		cfg.RunID = run.ID
		criteria, err := json.Marshal(cfg.Criteria)
		if err != nil {
			return fmt.Errorf("marshal criteria: %w", err)
		}
		_, err = tx.Exec(txCtx, `
			INSERT INTO smartorder.run_config (
				run_id, organization_id, criteria, tolerance_pct, default_quantity,
				max_budget, use_saving_products, use_ai_matching, criteria_defaulted,
				match_language, min_match_score
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);`,
			cfg.RunID, cfg.OrganizationID, criteria, cfg.TolerancePct, cfg.DefaultQuantity,
			cfg.MaxBudget, cfg.UseSavingProducts, cfg.UseAIMatching, cfg.CriteriaDefaulted,
			cfg.MatchLanguage, cfg.MinMatchScore)
		if err != nil {
			return fmt.Errorf("insert run config: %w", err)
		}
		return nil
	})
}

// GetRunByPublicID resolves a run by its external id, scoped to the caller's
// organisation. A run belonging to another organisation is Not Found, never
// Forbidden: existence is not disclosed.
func (r *Repository) GetRunByPublicID(ctx context.Context, orgID int64, publicID string) (*smartorder.Run, error) {
	var run *smartorder.Run
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+runColumns+` FROM smartorder.runs WHERE public_id = $1 AND organization_id = $2;`,
			publicID, orgID)
		var err error
		run, err = scanRun(row)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_run")
		}
		return err
	})
	return run, err
}

// GetRunByID is the internal-id equivalent, used by the worker.
func (r *Repository) GetRunByID(ctx context.Context, orgID, id int64) (*smartorder.Run, error) {
	var run *smartorder.Run
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+runColumns+` FROM smartorder.runs WHERE id = $1 AND organization_id = $2;`, id, orgID)
		var err error
		run, err = scanRun(row)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_run")
		}
		return err
	})
	return run, err
}

// ListRuns returns the buyer's history, newest first.
func (r *Repository) ListRuns(ctx context.Context, orgID int64, limit, offset int) ([]*smartorder.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []*smartorder.Run
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx,
			`SELECT `+runColumns+` FROM smartorder.runs
			 WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;`,
			orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			run, err := scanRun(rows)
			if err != nil {
				return err
			}
			out = append(out, run)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateRunStatus moves a run through its lifecycle.
func (r *Repository) UpdateRunStatus(ctx context.Context, id int64, status smartorder.RunStatus, step int, failureReason string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE smartorder.runs
			SET status = $2, current_step = $3, failure_reason = $4
			WHERE id = $1;`, id, string(status), step, failureReason)
		if err != nil {
			return err
		}

		if tag.RowsAffected() == 0 {
			return apperr.NotFound("smart_order_run")
		}
		return nil
	})
}

// ClaimRun atomically changes a queued run to processing. A false result means
// another worker or the inline runner won the race; it is not an error.
func (r *Repository) ClaimRun(ctx context.Context, orgID, runID int64) (bool, error) {
	var claimed bool
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE smartorder.runs
			SET status = 'processing', current_step = 1, failure_reason = ''
			WHERE id = $1 AND organization_id = $2 AND status = 'queued';`,
			runID, orgID)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}

// UpdateRunStats writes the counters, totals and AI telemetry after a pass.
func (r *Repository) UpdateRunStats(ctx context.Context, run *smartorder.Run) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE smartorder.runs SET
				total_rows = $2, matched_rows = $3, unmatched_rows = $4,
				no_supplier_rows = $5, coverage_blocked_rows = $6,
				institutional_blocked_rows = $7, below_min_qty_rows = $8,
				quota_blocked_rows = $9,
				estimated_total = $10, budget_exceeded = $11, budget_overage = $12,
				ai_calls = $13, ai_lines_adjudicated = $14, ai_cost_estimate = $15,
				ai_ceiling_hit = $16, deterministic_ms = $17, total_ms = $18,
				ai_lines_reviewed = $19, ai_lines_improved = $20, ai_cache_hits = $21
			WHERE id = $1;`,
			run.ID, run.Stats.TotalRows, run.Stats.MatchedRows, run.Stats.UnmatchedRows,
			run.Stats.NoSupplierRows, run.Stats.CoverageBlockedRows,
			run.Stats.InstitutionalBlockedRows, run.Stats.BelowMinQtyRows,
			run.Stats.QuotaBlockedRows,
			run.EstimatedTotal, run.BudgetExceeded, run.BudgetOverage,
			run.AI.Calls, run.AI.LinesAdjudicated, run.AI.CostEstimate,
			run.AI.CeilingHit, run.DeterministicMS, run.TotalMS,
			run.AI.LinesReviewed, run.AI.LinesImproved, run.AI.CacheHits)
		return err
	})
}

// FinalizeRun records the order, refusing if the run was already finalised.
//
// The `finalized_at IS NULL` predicate is part of the UPDATE rather than a
// preceding SELECT, so two concurrent submissions cannot both pass a check and
// both write. The loser sees zero rows affected and gets a conflict.
func (r *Repository) FinalizeRun(ctx context.Context, id, orderID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE smartorder.runs
			SET order_id = $2, finalized_at = now(), status = 'placed', current_step = 5
			WHERE id = $1 AND finalized_at IS NULL;`, id, orderID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("smartorder.already_finalized",
				"this smart order has already been placed")
		}
		return nil
	})
}
