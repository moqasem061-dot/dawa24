package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The cross-tenant warehouse listing.
//
// AsSystem throughout: an administrator is by definition reading rows that
// belong to every tenant but their own, and row-level security would otherwise
// return an empty page rather than an error, which reads as "there are no
// warehouses".

const adminWarehouseFrom = `
	FROM inventory.warehouses w
	LEFT JOIN org.organizations o ON o.id = w.organization_id
	LEFT JOIN org.branches b ON b.id = w.branch_id AND b.deleted_at IS NULL`

// ListAdminWarehouseRows returns one filtered page of every organisation's
// warehouses, with what each one is actually holding.
func (r *Repository) ListAdminWarehouseRows(
	ctx context.Context, f inventory.AdminWarehouseFilter,
) ([]*inventory.AdminWarehouseRow, int, error) {
	f.Normalize()

	where := []string{"w.deleted_at IS NULL"}
	args := make([]any, 0, 6)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.OrganizationID > 0 {
		where = append(where, "w.organization_id = "+arg(f.OrganizationID))
	}
	if f.BranchID > 0 {
		where = append(where, "w.branch_id = "+arg(f.BranchID))
	}
	if f.Active != nil {
		where = append(where, "COALESCE(w.is_active, false) = "+arg(*f.Active))
	}
	if f.Query != "" {
		p := arg("%" + f.Query + "%")
		where = append(where, fmt.Sprintf(`(
			w.name ILIKE %[1]s OR COALESCE(w.code, '') ILIKE %[1]s
			OR COALESCE(w.address, '') ILIKE %[1]s
			OR o.legal_name ILIKE %[1]s
			OR o.trade_name->>'ar' ILIKE %[1]s OR o.trade_name->>'en' ILIKE %[1]s
			OR o.name->>'ar' ILIKE %[1]s OR o.name->>'en' ILIKE %[1]s)`, p))
	}
	whereSQL := strings.Join(where, " AND ")

	var out []*inventory.AdminWarehouseRow
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) `+adminWarehouseFrom+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
			return fmt.Errorf("count admin warehouses: %w", err)
		}
		if total == 0 {
			return nil
		}

		paged := append(append([]any{}, args...), f.Limit, f.Offset)
		query := fmt.Sprintf(`
			SELECT w.id, w.public_id, w.organization_id, w.branch_id,
			       COALESCE(w.name, ''), COALESCE(w.code, ''), COALESCE(w.address, ''),
			       COALESCE(w.phone, ''), w.latitude, w.longitude,
			       COALESCE(w.is_active, false), w.created_at, w.updated_at,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''),
			                NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''),
			                o.legal_name, ''),
			       COALESCE(o.type, ''),
			       COALESCE(b.name->>'ar', b.name->>'en', ''),
			       COALESCE(st.items, 0), COALESCE(st.qty, 0), mv.last_movement
			%s
			LEFT JOIN LATERAL (
				SELECT count(*) AS items, COALESCE(SUM(s.quantity), 0) AS qty
				FROM inventory.stocks s
				WHERE s.warehouse_id = w.id AND s.deleted_at IS NULL
			) st ON true
			LEFT JOIN LATERAL (
				-- A movement records the stock row it moved, not the warehouse,
				-- so the warehouse has to be reached through it.
				SELECT MAX(m.created_at) AS last_movement
				FROM inventory.stock_movements m
				JOIN inventory.stocks ms ON ms.id = m.stock_id
				WHERE ms.warehouse_id = w.id
			) mv ON true
			WHERE %s
			ORDER BY st.qty DESC, w.id DESC
			LIMIT $%d OFFSET $%d`,
			adminWarehouseFrom, whereSQL, len(args)+1, len(args)+2)

		rows, err := tx.Query(txCtx, query, paged...)
		if err != nil {
			return fmt.Errorf("list admin warehouses: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var row inventory.AdminWarehouseRow
			if err := rows.Scan(
				&row.ID, &row.PublicID, &row.OrganizationID, &row.BranchID,
				&row.Name, &row.Code, &row.Address, &row.Phone,
				&row.Latitude, &row.Longitude,
				&row.IsActive, &row.CreatedAt, &row.UpdatedAt,
				&row.OrganizationName, &row.OrganizationType, &row.BranchName,
				&row.ItemCount, &row.TotalQuantity, &row.LastMovement,
			); err != nil {
				return fmt.Errorf("scan admin warehouse row: %w", err)
			}
			out = append(out, &row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inventory postgres: admin warehouse rows: %w", err)
	}
	return out, total, nil
}

// AdminWarehouseOwners lists only organisations that actually own a warehouse.
func (r *Repository) AdminWarehouseOwners(ctx context.Context) ([]inventory.AdminWarehouseOwner, error) {
	var out []inventory.AdminWarehouseOwner
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT o.id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.name->>'ar', ''),
			                NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'en', ''),
			                o.legal_name, ''),
			       COALESCE(o.type, ''), count(*)
			FROM inventory.warehouses w
			JOIN org.organizations o ON o.id = w.organization_id
			WHERE w.deleted_at IS NULL
			GROUP BY o.id, o.trade_name, o.name, o.legal_name, o.type
			ORDER BY 2;`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var owner inventory.AdminWarehouseOwner
			if err := rows.Scan(&owner.ID, &owner.Name, &owner.Type, &owner.Count); err != nil {
				return err
			}
			out = append(out, owner)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("inventory postgres: admin warehouse owners: %w", err)
	}
	return out, nil
}

// SetWarehouseActive enables or disables a warehouse on behalf of the platform.
//
// The audit row is written inside the same transaction as the change, so a
// privileged mutation cannot land without a record of who made it.
func (r *Repository) SetWarehouseActive(ctx context.Context, id int64, active bool) error {
	if id <= 0 {
		return nil
	}
	actorID := int64(0)
	if actor, ok := authctx.From(ctx); ok {
		actorID = actor.UserID
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID int64
		var was bool
		if err := tx.QueryRow(txCtx, `
			SELECT organization_id, COALESCE(is_active, false)
			FROM inventory.warehouses
			WHERE id = $1 AND deleted_at IS NULL;`, id).Scan(&orgID, &was); err != nil {
			if database.IsNotFound(err) {
				return nil
			}
			return err
		}
		if was == active {
			return nil // nothing changed; do not write an audit row saying it did
		}

		if _, err := tx.Exec(txCtx, `
			UPDATE inventory.warehouses
			SET is_active = $2, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;`, id, active); err != nil {
			return err
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			ActorUserID:    actorID,
			Action:         "warehouse.set_active",
			EntityType:     "inventory.warehouse",
			EntityID:       fmt.Sprintf("%d", id),
			Before:         map[string]any{"is_active": was},
			After:          map[string]any{"is_active": active},
		})
	})
}
