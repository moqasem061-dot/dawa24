package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetConfig loads the snapshot a run executed under.
func (r *Repository) GetConfig(ctx context.Context, runID int64) (*smartorder.Config, error) {
	var cfg smartorder.Config
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var criteria []byte
		err := tx.QueryRow(txCtx, `
			SELECT run_id, organization_id, criteria, tolerance_pct, default_quantity,
			       max_budget, use_saving_products, use_ai_matching, criteria_defaulted,
			       COALESCE(match_language, ''), COALESCE(min_match_score, 0)
			FROM smartorder.run_config WHERE run_id = $1;`, runID).Scan(
			&cfg.RunID, &cfg.OrganizationID, &criteria, &cfg.TolerancePct, &cfg.DefaultQuantity,
			&cfg.MaxBudget, &cfg.UseSavingProducts, &cfg.UseAIMatching, &cfg.CriteriaDefaulted,
			&cfg.MatchLanguage, &cfg.MinMatchScore)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_config")
		}
		if err != nil {
			return err
		}
		if len(criteria) > 0 {
			_ = json.Unmarshal(criteria, &cfg.Criteria)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetProfile returns the buyer's remembered defaults.
//
// A buyer who has never run a smart order has no row, which is not an error —
// they get the platform defaults instead.
func (r *Repository) GetProfile(ctx context.Context, orgID int64) (*smartorder.Profile, error) {
	p := &smartorder.Profile{
		OrganizationID: orgID,
		TolerancePct:   smartorder.DefaultTolerancePct,
	}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var criteria []byte
		err := tx.QueryRow(txCtx, `
			SELECT criteria, tolerance_pct, default_quantity,
			       use_saving_products, use_ai_matching, last_branch_id,
			       COALESCE(match_language, ''), COALESCE(min_match_score, 0)
			FROM smartorder.criteria_profiles WHERE organization_id = $1;`, orgID).Scan(
			&criteria, &p.TolerancePct, &p.DefaultQuantity,
			&p.UseSavingProducts, &p.UseAIMatching, &p.LastBranchID,
			&p.MatchLanguage, &p.MinMatchScore)
		if err == pgx.ErrNoRows {
			return nil // first run: platform defaults stand
		}
		if err != nil {
			return err
		}
		if len(criteria) > 0 {
			_ = json.Unmarshal(criteria, &p.Criteria)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// SaveProfile remembers the configuration for next time (FR-007).
func (r *Repository) SaveProfile(ctx context.Context, p *smartorder.Profile) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		criteria, err := json.Marshal(p.Criteria)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
			INSERT INTO smartorder.criteria_profiles (
				organization_id, criteria, tolerance_pct, default_quantity,
				use_saving_products, use_ai_matching, last_branch_id, match_language,
				min_match_score
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (organization_id) DO UPDATE SET
				criteria = EXCLUDED.criteria,
				tolerance_pct = EXCLUDED.tolerance_pct,
				default_quantity = EXCLUDED.default_quantity,
				use_saving_products = EXCLUDED.use_saving_products,
				use_ai_matching = EXCLUDED.use_ai_matching,
				last_branch_id = EXCLUDED.last_branch_id,
				match_language = EXCLUDED.match_language,
				min_match_score = EXCLUDED.min_match_score,
				updated_at = now();`,
			p.OrganizationID, criteria, p.TolerancePct, p.DefaultQuantity,
			p.UseSavingProducts, p.UseAIMatching, p.LastBranchID, p.MatchLanguage,
			p.MinMatchScore)
		return err
	})
}

// SaveMapping persists the column mapping, automatic guess included.
//
// The detected guess is kept alongside the confirmed mapping so a later question
// — "did the buyer override us, and where?" — is answerable without re-running
// detection against a file that may no longer exist.
func (r *Repository) SaveMapping(ctx context.Context, m *smartorder.Mapping) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		fields, err := json.Marshal(stringifyKeys(m.Fields))
		if err != nil {
			return err
		}
		detected, err := json.Marshal(stringifyKeys(m.Detected))
		if err != nil {
			return err
		}
		confidence, err := json.Marshal(m.Confidence)
		if err != nil {
			return err
		}
		confirmedAt := "NULL"
		_ = confirmedAt
		_, err = tx.Exec(txCtx, `
			INSERT INTO smartorder.column_mappings (
				run_id, organization_id, header_row, mapping, detected, confidence,
				user_overridden, confirmed_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7, CASE WHEN $8 THEN now() ELSE NULL END)
			ON CONFLICT (run_id) DO UPDATE SET
				header_row = EXCLUDED.header_row,
				mapping = EXCLUDED.mapping,
				detected = EXCLUDED.detected,
				confidence = EXCLUDED.confidence,
				user_overridden = EXCLUDED.user_overridden,
				confirmed_at = EXCLUDED.confirmed_at;`,
			m.RunID, m.OrganizationID, m.HeaderRow, fields, detected, confidence,
			m.UserOverridden, m.Confirmed)
		return err
	})
}

// GetMapping loads the confirmed mapping for a run.
func (r *Repository) GetMapping(ctx context.Context, runID int64) (*smartorder.Mapping, error) {
	m := &smartorder.Mapping{RunID: runID}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var fields, detected, confidence []byte
		var confirmedAt *string
		err := tx.QueryRow(txCtx, `
			SELECT organization_id, header_row, mapping, detected, confidence,
			       user_overridden, confirmed_at::text
			FROM smartorder.column_mappings WHERE run_id = $1;`, runID).Scan(
			&m.OrganizationID, &m.HeaderRow, &fields, &detected, &confidence,
			&m.UserOverridden, &confirmedAt)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_mapping")
		}
		if err != nil {
			return err
		}
		m.Fields = parseKeys(fields)
		m.Detected = parseKeys(detected)
		if len(confidence) > 0 {
			_ = json.Unmarshal(confidence, &m.Confidence)
		}
		m.Confirmed = confirmedAt != nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// JSONB object keys are strings; the mapping is keyed by column index. These two
// convert at the boundary rather than making the domain carry string keys.
func stringifyKeys(in map[int]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[itoa(k)] = v
	}
	return out
}

func parseKeys(raw []byte) map[int]string {
	if len(raw) == 0 {
		return map[int]string{}
	}
	var tmp map[string]string
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return map[int]string{}
	}
	out := make(map[int]string, len(tmp))
	for k, v := range tmp {
		if n, ok := atoi(k); ok {
			out[n] = v
		}
	}
	return out
}
