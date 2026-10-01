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
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ActivePolicy resolves an org-specific cohort first, then fallback to '*'.
func (r *Repository) ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error) {
	pol, _, err := r.ResolvePolicyAssignment(ctx, workflow, orgID)
	return pol, err
}

// ResolvePolicyAssignment resolves both the active policy and optional shadow version (US13 T151).
func (r *Repository) ResolvePolicyAssignment(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, string, error) {
	var bodyRaw []byte
	var shadowVer *string

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
			SELECT p.body, a.shadow_version
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

		err := tx.QueryRow(txCtx, query, workflow, cohorts, orgStr, orgPrefixed).Scan(&bodyRaw, &shadowVer)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})

	shadow := ""
	if shadowVer != nil {
		shadow = *shadowVer
	}

	if err != nil {
		return productmatch.DefaultPolicy(), shadow, err
	}

	if len(bodyRaw) == 0 {
		return productmatch.DefaultPolicy(), shadow, nil
	}

	var pol productmatch.Policy
	if err := json.Unmarshal(bodyRaw, &pol); err != nil {
		return productmatch.DefaultPolicy(), shadow, fmt.Errorf("unmarshal policy: %w", err)
	}

	return pol, shadow, nil
}

// GetPolicy retrieves a specific versioned policy by version ID.
func (r *Repository) GetPolicy(ctx context.Context, version string) (productmatch.Policy, error) {
	var bodyRaw []byte
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT body FROM catalog.match_policies WHERE version = $1;`
		return tx.QueryRow(txCtx, query, version).Scan(&bodyRaw)
	})
	if err != nil {
		return productmatch.Policy{}, fmt.Errorf("get policy %s: %w", version, err)
	}
	var pol productmatch.Policy
	if err := json.Unmarshal(bodyRaw, &pol); err != nil {
		return productmatch.Policy{}, fmt.Errorf("unmarshal policy %s: %w", version, err)
	}
	return pol, nil
}

// ListPolicies lists all registered match policies.
func (r *Repository) ListPolicies(ctx context.Context) ([]matching.PolicyRecord, error) {
	var records []matching.PolicyRecord
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT version, body, created_by, created_at FROM catalog.match_policies ORDER BY version ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rec matching.PolicyRecord
			var bodyRaw []byte
			if err := rows.Scan(&rec.Version, &bodyRaw, &rec.CreatedBy, &rec.CreatedAt); err != nil {
				return err
			}
			if len(bodyRaw) > 0 {
				_ = json.Unmarshal(bodyRaw, &rec.Body)
			}
			records = append(records, rec)
		}
		return nil
	})
	return records, err
}

// ListAssignments lists all policy assignments across workflows and cohorts.
func (r *Repository) ListAssignments(ctx context.Context) ([]matching.PolicyAssignment, error) {
	var assignments []matching.PolicyAssignment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT workflow, cohort, active_version, shadow_version, changed_by, changed_at
			FROM catalog.match_policy_assignments
			ORDER BY workflow ASC, cohort ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a matching.PolicyAssignment
			if err := rows.Scan(&a.Workflow, &a.Cohort, &a.ActiveVersion, &a.ShadowVersion, &a.ChangedBy, &a.ChangedAt); err != nil {
				return err
			}
			assignments = append(assignments, a)
		}
		return nil
	})
	return assignments, err
}

// UpdateAssignment updates or inserts a policy assignment and writes to the audit log (US13 T152).
func (r *Repository) UpdateAssignment(ctx context.Context, workflow, cohort, activeVersion string, shadowVersion *string, changedBy, reason string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Fetch current assignment if it exists
		var prevActive, prevShadow *string
		checkQuery := `SELECT active_version, shadow_version FROM catalog.match_policy_assignments WHERE workflow = $1 AND cohort = $2;`
		row := tx.QueryRow(txCtx, checkQuery, workflow, cohort)
		if err := row.Scan(&prevActive, &prevShadow); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read current assignment: %w", err)
		}

		// 2. Audit log
		logQuery := `
			INSERT INTO catalog.match_policy_assignment_log (
				workflow, cohort, previous_active_version, new_active_version,
				previous_shadow_version, new_shadow_version, changed_by, changed_at, reason
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8);
		`
		_, err := tx.Exec(txCtx, logQuery, workflow, cohort, prevActive, activeVersion, prevShadow, shadowVersion, changedBy, reason)
		if err != nil {
			return fmt.Errorf("write assignment log: %w", err)
		}

		// 3. Upsert assignment
		upsertQuery := `
			INSERT INTO catalog.match_policy_assignments (workflow, cohort, active_version, shadow_version, changed_by, changed_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (workflow, cohort) DO UPDATE SET
				active_version = EXCLUDED.active_version,
				shadow_version = EXCLUDED.shadow_version,
				changed_by = EXCLUDED.changed_by,
				changed_at = now();
		`
		_, err = tx.Exec(txCtx, upsertQuery, workflow, cohort, activeVersion, shadowVersion, changedBy)
		if err != nil {
			return fmt.Errorf("upsert assignment: %w", err)
		}
		return nil
	})
}

// RollbackAssignment restores the previous policy version for a workflow and cohort (US13 T152).
func (r *Repository) RollbackAssignment(ctx context.Context, workflow, cohort, changedBy string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Find the latest log entry with previous_active_version
		query := `
			SELECT previous_active_version, previous_shadow_version
			FROM catalog.match_policy_assignment_log
			WHERE workflow = $1 AND cohort = $2 AND previous_active_version IS NOT NULL
			ORDER BY id DESC
			LIMIT 1;
		`
		var prevActive, prevShadow *string
		err := tx.QueryRow(txCtx, query, workflow, cohort).Scan(&prevActive, &prevShadow)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.New("no previous policy version available for rollback")
			}
			return fmt.Errorf("query rollback target: %w", err)
		}
		if prevActive == nil || *prevActive == "" {
			return errors.New("previous active policy version is empty")
		}

		// 2. Fetch current assignment
		var curActive, curShadow *string
		_ = tx.QueryRow(txCtx, `SELECT active_version, shadow_version FROM catalog.match_policy_assignments WHERE workflow = $1 AND cohort = $2;`,
			workflow, cohort).Scan(&curActive, &curShadow)

		reason := fmt.Sprintf("Rollback to active=%s", *prevActive)
		if prevShadow != nil {
			reason += fmt.Sprintf(", shadow=%s", *prevShadow)
		}

		// 3. Insert rollback audit entry
		logQuery := `
			INSERT INTO catalog.match_policy_assignment_log (
				workflow, cohort, previous_active_version, new_active_version,
				previous_shadow_version, new_shadow_version, changed_by, changed_at, reason
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8);
		`
		_, err = tx.Exec(txCtx, logQuery, workflow, cohort, curActive, *prevActive, curShadow, prevShadow, changedBy, reason)
		if err != nil {
			return fmt.Errorf("write rollback audit: %w", err)
		}

		// 4. Update assignment
		updQuery := `
			UPDATE catalog.match_policy_assignments
			SET active_version = $1, shadow_version = $2, changed_by = $3, changed_at = now()
			WHERE workflow = $4 AND cohort = $5;
		`
		tag, err := tx.Exec(txCtx, updQuery, *prevActive, prevShadow, changedBy, workflow, cohort)
		if err != nil {
			return fmt.Errorf("update assignment on rollback: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errors.New("assignment row not found during rollback")
		}
		return nil
	})
}
