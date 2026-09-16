package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Trash operates across every tenant's rows by definition — it is the platform
// admin's recovery tool — so all of it runs AsSystem. The route is gated by
// RequirePagePermission, which is what keeps it away from non-staff.

// ListSoftDeletableTables discovers every table carrying a deleted_at column
// and counts its rows. Discovery beats a hardcoded list: a new soft-deletable
// table appears here without anyone remembering to register it.
func (r *Repository) ListSoftDeletableTables(ctx context.Context) ([]*platformadmin.TrashModel, error) {
	var models []*platformadmin.TrashModel
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const discover = `
			SELECT c.table_schema, c.table_name
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			JOIN information_schema.columns c_id
			  ON c_id.table_schema = c.table_schema AND c_id.table_name = c.table_name AND c_id.column_name = 'id'
			WHERE c.column_name = 'deleted_at'
			  AND t.table_type = 'BASE TABLE'
			  AND c.table_schema NOT IN ('pg_catalog', 'information_schema')
			ORDER BY c.table_schema, c.table_name;
		`
		rows, err := tx.Query(txCtx, discover)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m platformadmin.TrashModel
			if err := rows.Scan(&m.Schema, &m.Table); err != nil {
				return err
			}
			m.Key = m.Schema + "." + m.Table
			models = append(models, &m)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if len(models) == 0 {
			return nil
		}

		// Counts are real queries. The screen used to display invented numbers
		// (1240 products, 14200 orders) that no query produced.
		//
		// All of them in ONE statement rather than one statement per table.
		// This schema has a hundred tables carrying deleted_at, so the loop this
		// replaces issued a hundred sequential round trips to render a single
		// admin page — and each one was a full COUNT(*), so the page also read
		// every row of every table in the database, one table at a time, with a
		// network round trip between each.
		//
		// UNION ALL of a hundred small aggregates is one round trip and lets
		// PostgreSQL run the counts back to back without waiting for us.
		// Identifiers come from information_schema above, never from user input,
		// so quoting them with %q is safe.
		var b strings.Builder
		for i, m := range models {
			if i > 0 {
				b.WriteString("\nUNION ALL\n")
			}
			fmt.Fprintf(&b,
				`SELECT %d AS idx, COUNT(*) AS total, COUNT(*) FILTER (WHERE deleted_at IS NOT NULL) AS trashed FROM %q.%q`,
				i, m.Schema, m.Table)
		}

		countRows, err := tx.Query(txCtx, b.String())
		if err != nil {
			return fmt.Errorf("count trash models: %w", err)
		}
		defer countRows.Close()

		for countRows.Next() {
			var idx int
			var total, trashed int64
			if err := countRows.Scan(&idx, &total, &trashed); err != nil {
				return fmt.Errorf("scan trash counts: %w", err)
			}
			// Defensive: a UNION ALL has no ordering guarantee, so the row is
			// placed by the index it carries rather than by arrival order.
			if idx < 0 || idx >= len(models) {
				continue
			}
			models[idx].TotalCount = total
			models[idx].TrashedRows = trashed
		}
		return countRows.Err()
	})
	return models, err
}

// ListTrashedRows returns the soft-deleted rows of one table with a readable
// label taken from whichever of the usual name columns the table has.
func (r *Repository) ListTrashedRows(ctx context.Context, schema, table string, limit, offset int) ([]*platformadmin.TrashRow, error) {
	rows, _, err := r.ListTrashedRowsWithTotal(ctx, schema, table, "", limit, offset)
	return rows, err
}

// ListTrashedRowsWithTotal returns the soft-deleted rows of one table with total count and human identity details.
func (r *Repository) ListTrashedRowsWithTotal(ctx context.Context, schema, table, search string, limit, offset int) ([]*platformadmin.TrashRow, int, error) {
	var out []*platformadmin.TrashRow
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := assertSoftDeletable(txCtx, tx, schema, table); err != nil {
			return err
		}

		labelExpr, err := trashLabelExpr(txCtx, tx, schema, table)
		if err != nil {
			return err
		}
		codeExpr, err := trashCodeExpr(txCtx, tx, schema, table)
		if err != nil {
			return err
		}

		hasOrg, err := columnExists(txCtx, tx, schema, table, "organization_id")
		if err != nil {
			return err
		}
		hasProduct, err := columnExists(txCtx, tx, schema, table, "product_id")
		if err != nil {
			return err
		}
		hasDeletedBy, err := columnExists(txCtx, tx, schema, table, "deleted_by")
		if err != nil {
			return err
		}

		whereClauses := []string{"t.deleted_at IS NOT NULL"}
		var countArgs []any

		if search != "" {
			pattern := "%" + search + "%"
			countArgs = append(countArgs, pattern)
			pIdx := fmt.Sprintf("$%d", len(countArgs))
			searchParts := []string{
				fmt.Sprintf("%s ILIKE %s", labelExpr, pIdx),
			}
			if codeExpr != "''" {
				searchParts = append(searchParts, fmt.Sprintf("%s ILIKE %s", codeExpr, pIdx))
			}
			if hasOrg {
				searchParts = append(searchParts, fmt.Sprintf("(o.legal_name ILIKE %s OR o.trade_name::text ILIKE %s)", pIdx, pIdx))
			}
			whereClauses = append(whereClauses, "("+strings.Join(searchParts, " OR ")+")")
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		var joinSQL strings.Builder
		if hasOrg {
			joinSQL.WriteString(` LEFT JOIN org.organizations o ON o.id = t.organization_id `)
		}
		if hasProduct && (schema == "catalog" || table == "product_variants") {
			joinSQL.WriteString(` LEFT JOIN catalog.products p ON p.id = t.product_id `)
		}
		const userNameTextExpr = `COALESCE(CASE WHEN jsonb_typeof(to_jsonb(%s.name)) = 'object' THEN COALESCE(to_jsonb(%s.name)->>'ar', to_jsonb(%s.name)->>'en') ELSE NULLIF(%s.name::text, 'null') END, %s.email, '')`

		if hasDeletedBy {
			joinSQL.WriteString(` LEFT JOIN identity.users u_del ON u_del.id = t.deleted_by `)
		} else {
			entityTypeLiteral := fmt.Sprintf("'%s.%s'", strings.ReplaceAll(schema, "'", "''"), strings.ReplaceAll(table, "'", "''"))
			auditUserExpr := fmt.Sprintf(userNameTextExpr, "u_audit", "u_audit", "u_audit", "u_audit", "u_audit")
			fmt.Fprintf(&joinSQL, ` LEFT JOIN LATERAL (
				SELECT a.actor_user_id, %s AS audit_user_name
				FROM platform.audit_log a
				LEFT JOIN identity.users u_audit ON u_audit.id = a.actor_user_id
				WHERE a.entity_type = %s AND a.entity_id = t.id::text
				ORDER BY a.created_at DESC LIMIT 1
			) al ON true `, auditUserExpr, entityTypeLiteral)
		}

		countQ := fmt.Sprintf(`SELECT count(*) FROM %q.%q t %s WHERE %s`, schema, table, joinSQL.String(), whereSQL)
		if err := tx.QueryRow(txCtx, countQ, countArgs...).Scan(&total); err != nil {
			return err
		}

		orgIDExpr := "NULL::bigint"
		orgNameExpr := "''::text"
		orgDelExpr := "false"
		if hasOrg {
			orgIDExpr = "t.organization_id"
			orgNameExpr = "COALESCE(CASE WHEN jsonb_typeof(to_jsonb(o.trade_name)) = 'object' THEN to_jsonb(o.trade_name)->>'ar' ELSE o.trade_name::text END, o.legal_name, '')"
			orgDelExpr = "COALESCE(o.deleted_at IS NOT NULL, false)"
		}

		prodDelExpr := "false"
		if hasProduct && (schema == "catalog" || table == "product_variants") {
			prodDelExpr = "COALESCE(p.deleted_at IS NOT NULL, false)"
		}

		delByIDExpr := "NULL::bigint"
		delByNameExpr := "''::text"
		if hasDeletedBy {
			delByIDExpr = "t.deleted_by"
			delByNameExpr = fmt.Sprintf(userNameTextExpr, "u_del", "u_del", "u_del", "u_del", "u_del")
		} else {
			delByIDExpr = "al.actor_user_id"
			delByNameExpr = "COALESCE(al.audit_user_name, '')"
		}

		dataArgs := append([]any{}, countArgs...)
		dataArgs = append(dataArgs, limit, offset)
		limIdx := fmt.Sprintf("$%d", len(dataArgs)-1)
		offIdx := fmt.Sprintf("$%d", len(dataArgs))

		dataQ := fmt.Sprintf(`
			SELECT
				t.id,
				%s AS label,
				%s AS code,
				%s AS org_id,
				%s AS org_name,
				%s AS org_deleted,
				%s AS parent_prod_deleted,
				%s AS deleted_by,
				%s AS deleted_by_name,
				to_char(t.deleted_at, 'YYYY-MM-DD HH24:MI') AS deleted_at
			FROM %q.%q t
			%s
			WHERE %s
			ORDER BY t.deleted_at DESC, t.id DESC
			LIMIT %s OFFSET %s`,
			labelExpr, codeExpr,
			orgIDExpr, orgNameExpr, orgDelExpr, prodDelExpr,
			delByIDExpr, delByNameExpr,
			schema, table,
			joinSQL.String(),
			whereSQL,
			limIdx, offIdx,
		)

		rows, err := tx.Query(txCtx, dataQ, dataArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row platformadmin.TrashRow
			var orgDeleted, parentProdDeleted bool
			if err := rows.Scan(
				&row.ID,
				&row.Label,
				&row.Code,
				&row.OrganizationID,
				&row.OrganizationName,
				&orgDeleted,
				&parentProdDeleted,
				&row.DeletedBy,
				&row.DeletedByName,
				&row.DeletedAt,
			); err != nil {
				return err
			}

			if orgDeleted {
				row.CanRestore = false
				row.CannotRestoreWhy = "المنشأة التابعة لها محذوفة حالياً — يجب استرجاع المنشأة أولاً"
			} else if parentProdDeleted {
				row.CanRestore = false
				row.CannotRestoreWhy = "المنتج الأساسي محذوف حالياً — يجب استرجاع المنتج أولاً"
			} else {
				row.CanRestore = true
			}

			out = append(out, &row)
		}
		return rows.Err()
	})
	return out, total, err
}

// RestoreTrashedRow clears deleted_at, refusing when the row's parent
// organization is itself still deleted — restoring a child under a deleted
// parent produces a row nothing can reach.
func (r *Repository) RestoreTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := assertSoftDeletable(txCtx, tx, schema, table); err != nil {
			return err
		}

		hasOrg, err := columnExists(txCtx, tx, schema, table, "organization_id")
		if err != nil {
			return err
		}
		if hasOrg {
			var orphaned bool
			q := fmt.Sprintf(`
				SELECT EXISTS (
					SELECT 1 FROM %q.%q t
					JOIN org.organizations o ON o.id = t.organization_id
					WHERE t.id = $1 AND o.deleted_at IS NOT NULL
				)`, schema, table)
			if err := tx.QueryRow(txCtx, q, id).Scan(&orphaned); err != nil {
				return err
			}
			if orphaned {
				return apperr.Conflict("trash.parent_deleted",
					"Restore the owning organization first; this record's organization is still deleted.")
			}
		}

		q := fmt.Sprintf(`UPDATE %q.%q SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL`, schema, table)
		tag, err := tx.Exec(txCtx, q, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("trashed_row")
		}
		return writeTrashAudit(txCtx, tx, "restore", schema, table, id, actorID, "")
	})
}

// PurgeTrashedRow hard-deletes a row. Irreversible, so the row is serialised
// into the audit log before it goes.
func (r *Repository) PurgeTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := assertSoftDeletable(txCtx, tx, schema, table); err != nil {
			return err
		}

		var snapshot string
		snapQ := fmt.Sprintf(`SELECT to_jsonb(t)::text FROM %q.%q t WHERE t.id = $1 AND t.deleted_at IS NOT NULL`, schema, table)
		if err := tx.QueryRow(txCtx, snapQ, id).Scan(&snapshot); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("trashed_row")
			}
			return err
		}

		q := fmt.Sprintf(`DELETE FROM %q.%q WHERE id = $1 AND deleted_at IS NOT NULL`, schema, table)
		tag, err := tx.Exec(txCtx, q, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("trashed_row")
		}
		return writeTrashAudit(txCtx, tx, "purge", schema, table, id, actorID, snapshot)
	})
}
