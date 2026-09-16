package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListAuditLogByOrg returns audit trail entries filtered to a specific organization.
func (r *Repository) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, error) {
	list, _, err := r.ListAuditLogByOrgWithTotal(ctx, orgID, limit, offset)
	return list, err
}

// ListAuditLogByOrgWithTotal returns paginated audit trail entries filtered to a specific organization with total count.
func (r *Repository) ListAuditLogByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, int, error) {
	var list []*platformadmin.AuditEntry
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM platform.audit_log WHERE organization_id = $1;`, orgID).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), '') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, '') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			WHERE a.organization_id = $1
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			enrichAuditEntry(&e)
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListOrgStaffAuditLogWithTotal returns paginated audit trail entries strictly performed by organization employees/members.
func (r *Repository) ListOrgStaffAuditLogWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, int, error) {
	var list []*platformadmin.AuditEntry
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `
			SELECT count(*)
			FROM platform.audit_log a
			JOIN identity.users u ON a.actor_user_id = u.id
			WHERE a.organization_id = $1
			  AND a.actor_user_id IN (
			      SELECT user_id FROM org.members WHERE organization_id = $1
			      UNION
			      SELECT owner_id FROM org.organizations WHERE id = $1
			  )
			  AND (u.role IS NULL OR u.role NOT IN ('super_admin', 'platform_admin', 'admin_staff', 'staff'));
		`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), '') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, '') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			WHERE a.organization_id = $1
			  AND a.actor_user_id IN (
			      SELECT user_id FROM org.members WHERE organization_id = $1
			      UNION
			      SELECT owner_id FROM org.organizations WHERE id = $1
			  )
			  AND (u.role IS NULL OR u.role NOT IN ('super_admin', 'platform_admin', 'admin_staff', 'staff'))
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			enrichAuditEntry(&e)
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListAuditLogWithFilter returns filtered audit log entries and total count matching the criteria.
func (r *Repository) ListAuditLogWithFilter(ctx context.Context, filter platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	if filter.Limit <= 0 || filter.Limit > 10000 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var list []*platformadmin.AuditEntry
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var conditions []string
		var args []any
		argIdx := 1

		if filter.OrganizationID != nil && *filter.OrganizationID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.organization_id = $%d", argIdx))
			args = append(args, *filter.OrganizationID)
			argIdx++
		}

		if filter.ActorUserID != nil && *filter.ActorUserID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.actor_user_id = $%d", argIdx))
			args = append(args, *filter.ActorUserID)
			argIdx++
		}

		if strings.TrimSpace(filter.Action) != "" {
			conditions = append(conditions, fmt.Sprintf("(a.action ILIKE '%%' || $%d || '%%' OR a.entity_type ILIKE '%%' || $%d || '%%')", argIdx, argIdx))
			args = append(args, strings.TrimSpace(filter.Action))
			argIdx++
		}

		if strings.TrimSpace(filter.EntityType) != "" {
			conditions = append(conditions, fmt.Sprintf("a.entity_type = $%d", argIdx))
			args = append(args, strings.TrimSpace(filter.EntityType))
			argIdx++
		}

		if filter.DateFrom != nil {
			conditions = append(conditions, fmt.Sprintf("a.created_at >= $%d", argIdx))
			args = append(args, *filter.DateFrom)
			argIdx++
		}

		if filter.DateTo != nil {
			conditions = append(conditions, fmt.Sprintf("a.created_at < $%d", argIdx))
			args = append(args, filter.DateTo.Add(24*time.Hour))
			argIdx++
		}

		if strings.TrimSpace(filter.Search) != "" {
			s := strings.TrimSpace(filter.Search)
			conditions = append(conditions, fmt.Sprintf("(u.name->>'ar' ILIKE '%%' || $%d || '%%' OR u.name->>'en' ILIKE '%%' || $%d || '%%' OR u.email ILIKE '%%' || $%d || '%%' OR o.trade_name->>'ar' ILIKE '%%' || $%d || '%%' OR a.action ILIKE '%%' || $%d || '%%' OR a.entity_id ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, s)
			argIdx++
		}

		whereClause := ""
		if len(conditions) > 0 {
			whereClause = "WHERE " + strings.Join(conditions, " AND ")
		}

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			%s;
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		query := fmt.Sprintf(`
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), '') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, '') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			%s
			ORDER BY a.created_at DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, argIdx, argIdx+1)

		args = append(args, filter.Limit, filter.Offset)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			enrichAuditEntry(&e)
			list = append(list, &e)
		}
		return rows.Err()
	})

	return list, total, err
}

// GetAuditEntryByID reads one audit entry with its before and after states.
//
// The screen used to embed every entry's diff into the page as escaped JSON,
// so the detail modal never parsed. Fetching one on demand is what the modal
// needs and is cheaper than shipping a page of diffs nobody opens.
func (r *Repository) GetAuditEntryByID(ctx context.Context, id int64) (*platformadmin.AuditEntry, error) {
	if id <= 0 {
		return nil, nil
	}
	var e platformadmin.AuditEntry
	var beforeJSON, afterJSON []byte
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''),
			                o.legal_name, ''),
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), ''),
			       COALESCE(u.email::text, ''),
			       COALESCE(a.action, ''), COALESCE(a.entity_type, ''),
			       COALESCE(a.entity_id, ''), a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN org.organizations o ON o.id = a.organization_id
			LEFT JOIN identity.users u ON u.id = a.actor_user_id
			WHERE a.id = $1;`, id).Scan(
			&e.ID, &e.OrganizationID, &e.OrganizationName,
			&e.ActorUserID, &e.ActorName, &e.ActorEmail,
			&e.Action, &e.EntityType, &e.EntityID,
			&beforeJSON, &afterJSON, &e.CreatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform_admin postgres: get audit entry: %w", err)
	}
	// A diff that cannot be decoded is left empty rather than failing the read:
	// the rest of the entry still answers who did what and when.
	if len(beforeJSON) > 0 {
		_ = json.Unmarshal(beforeJSON, &e.Before)
	}
	if len(afterJSON) > 0 {
		_ = json.Unmarshal(afterJSON, &e.After)
	}
	return &e, nil
}
