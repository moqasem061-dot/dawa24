package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

const selectAIConfigsQuery = `
	SELECT id, gateway_route, prompt_version, contract_version, native_structured,
	       bytes_per_token::text, max_items_per_attempt, max_input_tokens, max_output_tokens,
	       max_cost_per_attempt::NUMERIC(12,2), qualified_cohorts, status, qualification_report_uri,
	       created_by, created_at, updated_by, updated_at
	FROM catalog.ai_match_configurations
`

// ListAIConfigurations returns all registered AI matching configurations (US10 T134).
func (r *Repository) ListAIConfigurations(ctx context.Context) ([]matching.AIConfiguration, error) {
	var list []matching.AIConfiguration

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := selectAIConfigsQuery + ` ORDER BY id ASC`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cfg, err := scanAIConfiguration(rows)
			if err != nil {
				return err
			}
			list = append(list, cfg)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list ai configurations: %w", err)
	}

	return list, nil
}

// GetAIConfiguration retrieves an AI configuration by ID (US10 T134).
func (r *Repository) GetAIConfiguration(ctx context.Context, id string) (*matching.AIConfiguration, error) {
	var cfg *matching.AIConfiguration

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := selectAIConfigsQuery + ` WHERE id = $1 LIMIT 1`
		row := tx.QueryRow(txCtx, query, id)
		c, err := scanAIConfiguration(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		cfg = &c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get ai configuration %q: %w", id, err)
	}

	return cfg, nil
}

// UpdateAIConfigurationStatus transitions an AI configuration status with audit logging (US10 T134).
func (r *Repository) UpdateAIConfigurationStatus(ctx context.Context, id string, newStatus string, changedBy string, reason string) error {
	switch newStatus {
	case "candidate", "shadow", "qualified", "disabled":
		// valid
	default:
		return fmt.Errorf("invalid status %q: must be candidate, shadow, qualified, or disabled", newStatus)
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var oldStatus string
		err := tx.QueryRow(txCtx, `SELECT status FROM catalog.ai_match_configurations WHERE id = $1 FOR UPDATE`, id).Scan(&oldStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("configuration %q not found", id)
		}
		if err != nil {
			return fmt.Errorf("lock configuration %q: %w", id, err)
		}

		_, err = tx.Exec(txCtx, `
			UPDATE catalog.ai_match_configurations
			SET status = $2, updated_by = $3, updated_at = now()
			WHERE id = $1
		`, id, newStatus, changedBy)
		if err != nil {
			return fmt.Errorf("update status for %q: %w", id, err)
		}

		_, err = tx.Exec(txCtx, `
			INSERT INTO catalog.ai_match_configuration_log (config_id, old_status, new_status, changed_by, reason)
			VALUES ($1, $2, $3, $4, $5)
		`, id, oldStatus, newStatus, changedBy, reason)
		if err != nil {
			return fmt.Errorf("insert audit log for %q: %w", id, err)
		}

		return nil
	})
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAIConfiguration(s rowScanner) (matching.AIConfiguration, error) {
	var (
		cfg         matching.AIConfiguration
		bptStr      string
		cohortBytes []byte
	)

	err := s.Scan(
		&cfg.ID,
		&cfg.GatewayRoute,
		&cfg.PromptVersion,
		&cfg.ContractVersion,
		&cfg.NativeStructured,
		&bptStr,
		&cfg.MaxItemsPerAttempt,
		&cfg.MaxInputTokens,
		&cfg.MaxOutputTokens,
		&cfg.MaxCostPerAttempt,
		&cohortBytes,
		&cfg.Status,
		&cfg.QualificationReportURI,
		&cfg.CreatedBy,
		&cfg.CreatedAt,
		&cfg.UpdatedBy,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return matching.AIConfiguration{}, err
	}

	if bpt, err := strconv.ParseFloat(bptStr, 64); err == nil {
		cfg.BytesPerToken = bpt
	} else {
		cfg.BytesPerToken = 4.0
	}

	if len(cohortBytes) > 0 {
		_ = json.Unmarshal(cohortBytes, &cfg.QualifiedCohorts)
	}

	return cfg, nil
}
