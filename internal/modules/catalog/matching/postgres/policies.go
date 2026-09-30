package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ActivePolicy resolves an org-specific cohort first, then fallback to '*'.
func (r *Repository) ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error) {
	var bodyRaw []byte

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cohorts := []string{"*"}
		if orgID > 0 {
			cohorts = []string{
				strconv.FormatInt(orgID, 10),
				fmt.Sprintf("org:%d", orgID),
				"*",
			}
		}

		query := `
			SELECT p.body
			FROM catalog.match_policy_assignments a
			JOIN catalog.match_policies p ON p.version = a.active_version
			WHERE a.workflow = $1 AND a.cohort = ANY($2)
			ORDER BY CASE
				WHEN a.cohort = $3 THEN 1
				WHEN a.cohort = $4 THEN 2
				ELSE 3
			END
			LIMIT 1
		`

		var orgStr, orgPrefixed string
		if orgID > 0 {
			orgStr = strconv.FormatInt(orgID, 10)
			orgPrefixed = fmt.Sprintf("org:%d", orgID)
		}

		err := tx.QueryRow(txCtx, query, workflow, cohorts, orgStr, orgPrefixed).Scan(&bodyRaw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return productmatch.DefaultPolicy(), err
	}

	if len(bodyRaw) == 0 {
		return productmatch.DefaultPolicy(), nil
	}

	var pol productmatch.Policy
	if err := json.Unmarshal(bodyRaw, &pol); err != nil {
		return productmatch.DefaultPolicy(), fmt.Errorf("unmarshal policy: %w", err)
	}

	return pol, nil
}
