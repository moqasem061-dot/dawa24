package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListMatchDecisionsFiltered returns paginated decision memories matching all specified filters.
func (r *Repository) ListMatchDecisionsFiltered(ctx context.Context, f catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	var out []*catalog.MatchDecisionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"1=1"}
		var args []any

		if search := strings.TrimSpace(f.Search); search != "" {
			args = append(args, "%"+search+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+p+" OR m.reason ILIKE "+p+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+p+" OR COALESCE(p.sku, '') ILIKE "+p+")")
		}
		if f.OrganizationID != nil && *f.OrganizationID > 0 {
			args = append(args, *f.OrganizationID)
			where = append(where, "m.organization_id = $"+strconv.Itoa(len(args)))
		}
		if f.UserID != nil && *f.UserID > 0 {
			args = append(args, *f.UserID)
			where = append(where, "m.user_id = $"+strconv.Itoa(len(args)))
		}
		if f.Scope != "" {
			args = append(args, f.Scope)
			where = append(where, "m.scope = $"+strconv.Itoa(len(args)))
		}
		if f.Source != "" {
			args = append(args, f.Source)
			where = append(where, "m.source = $"+strconv.Itoa(len(args)))
		}
		if f.MinConfidence != nil {
			args = append(args, *f.MinConfidence)
			where = append(where, "m.confidence >= $"+strconv.Itoa(len(args)))
		}
		if f.MaxConfidence != nil {
			args = append(args, *f.MaxConfidence)
			where = append(where, "m.confidence <= $"+strconv.Itoa(len(args)))
		}
		if f.OnlyUnlinked {
			where = append(where, "m.chosen_product_id IS NULL")
		}
		if f.CreatedFrom != nil {
			args = append(args, *f.CreatedFrom)
			where = append(where, "m.created_at >= $"+strconv.Itoa(len(args)))
		}
		if f.CreatedTo != nil {
			args = append(args, *f.CreatedTo)
			where = append(where, "m.created_at <= $"+strconv.Itoa(len(args)))
		}
		if f.LastUsedFrom != nil {
			args = append(args, *f.LastUsedFrom)
			where = append(where, "m.last_used_at >= $"+strconv.Itoa(len(args)))
		}
		if f.LastUsedTo != nil {
			args = append(args, *f.LastUsedTo)
			where = append(where, "m.last_used_at <= $"+strconv.Itoa(len(args)))
		}
		if f.MinHitCount != nil {
			args = append(args, *f.MinHitCount)
			where = append(where, "m.hit_count >= $"+strconv.Itoa(len(args)))
		}
		if strings.TrimSpace(f.PromptVersion) != "" {
			args = append(args, strings.TrimSpace(f.PromptVersion))
			where = append(where, "m.prompt_version = $"+strconv.Itoa(len(args)))
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, f.Limit, f.Offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.confidence, COALESCE(m.reason, ''), m.prompt_version, m.hit_count,
			       m.scope, m.source, m.promoted_by, m.promoted_at,
			       m.created_at, m.last_used_at,
			       COALESCE(o.name->>'ar', o.name->>'en', ''), COALESCE(u.name->>'ar', u.name->>'en', u.email, '')
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			LEFT JOIN org.organizations o ON o.id = m.organization_id
			LEFT JOIN identity.users u ON u.id = m.user_id
			WHERE ` + clause + `
			ORDER BY m.last_used_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID, orgID, userID, promBy *int64
			var reason, orgName, userName string
			if err := rows.Scan(
				&v.ID, &orgID, &userID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.Scope, &v.Source, &promBy, &v.PromotedAt,
				&v.CreatedAt, &v.LastUsedAt,
				&orgName, &userName,
			); err != nil {
				return err
			}
			v.OrganizationID = orgID
			v.UserID = userID
			v.ChosenProductID = chosenID
			v.Reason = reason
			v.PromotedBy = promBy
			v.OrganizationName = orgName
			v.UserName = userName
			out = append(out, &v)
		}
		return rows.Err()
	})
	return out, total, err
}

// ListMatchDecisionsForOrgWithPlatform returns decisions visible to an organization (own + platform-inherited).
// On key collision, the organization's own decision overrides the platform row.
func (r *Repository) ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	pref, err := r.GetDecisionMemoryPreference(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	if !pref {
		return r.ListMatchDecisionsForOrg(ctx, orgID, search, limit, offset)
	}

	var out []*catalog.MatchDecisionView
	var total int

	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"(m.organization_id = $1 OR m.scope = 'platform')"}
		args := []any{orgID}

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+p+" OR m.reason ILIKE "+p+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+p+" OR COALESCE(p.sku, '') ILIKE "+p+")")
		}
		clause := strings.Join(where, " AND ")

		countQuery := fmt.Sprintf(`
			WITH filtered AS (
				SELECT m.id,
				       ROW_NUMBER() OVER (
				           PARTITION BY m.decision_key
				           ORDER BY CASE WHEN m.organization_id = $1 THEN 0 ELSE 1 END, m.last_used_at DESC
				       ) as rn
				FROM catalog.match_decisions m
				LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
				WHERE %s
			)
			SELECT count(*) FROM filtered WHERE rn = 1;
		`, clause)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := fmt.Sprintf(`
			WITH ranked AS (
				SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
				       COALESCE(p.name->>'ar', p.name->>'en', '') AS prod_name,
				       COALESCE(p.sku, '') AS prod_sku,
				       m.confidence, COALESCE(m.reason, '') AS reason, m.prompt_version, m.hit_count,
				       m.scope, m.source, m.promoted_by, m.promoted_at,
				       m.created_at, m.last_used_at,
				       (m.organization_id IS NULL OR m.organization_id <> $1) AS is_inherited,
				       ROW_NUMBER() OVER (
				           PARTITION BY m.decision_key
				           ORDER BY CASE WHEN m.organization_id = $1 THEN 0 ELSE 1 END, m.last_used_at DESC
				       ) as rn
				FROM catalog.match_decisions m
				LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
				WHERE %s
			)
			SELECT id, organization_id, user_id, decision_key, norm_name, chosen_product_id,
			       prod_name, prod_sku, confidence, reason, prompt_version, hit_count,
			       scope, source, promoted_by, promoted_at, created_at, last_used_at, is_inherited
			FROM ranked
			WHERE rn = 1
			ORDER BY last_used_at DESC, id DESC
			LIMIT %s OFFSET %s;
		`, clause, limParam, offParam)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID, dbOrgID, dbUserID, promBy *int64
			var reason string
			if err := rows.Scan(
				&v.ID, &dbOrgID, &dbUserID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.Scope, &v.Source, &promBy, &v.PromotedAt,
				&v.CreatedAt, &v.LastUsedAt, &v.IsPlatformInherited,
			); err != nil {
				return err
			}
			v.OrganizationID = dbOrgID
			v.UserID = dbUserID
			v.ChosenProductID = chosenID
			v.Reason = reason
			v.PromotedBy = promBy
			out = append(out, &v)
		}
		return rows.Err()
	})
	return out, total, err
}
