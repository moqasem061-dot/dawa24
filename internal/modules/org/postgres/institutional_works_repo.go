package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetInstitutionalWorkByID retrieves an institutional work by its primary key along with connected entities.
func (r *Repository) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	var iw org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id,
			       COALESCE(p.title->>'ar', p.title->>'en', '') AS parent_title,
			       COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id), 0) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE iw.id = $1 AND iw.deleted_at IS NULL;
		`
		var pricingStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&iw.ID, &iw.PublicID, &iw.Title, &iw.Description, &iw.Icon, &pricingStr,
			&iw.IsActive, &iw.ViewType, &iw.Slug, &iw.ParentID, &iw.ParentTitle,
			&iw.BranchCount, &iw.CreatedAt, &iw.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("institutional_work")
			}
			return err
		}
		iw.PricingType = org.PricingType(pricingStr)

		// Load connections
		connRows, err := tx.Query(txCtx, `
			SELECT iwc.to_institutional_work_id,
			       COALESCE(target.title->>'ar', target.title->>'en', '')
			FROM org.institutional_work_connections iwc
			JOIN org.institutional_works target ON iwc.to_institutional_work_id = target.id
			WHERE iwc.from_institutional_work_id = $1 AND target.deleted_at IS NULL
			ORDER BY target.id ASC;
		`, id)
		if err == nil {
			defer connRows.Close()
			for connRows.Next() {
				var toID int64
				var toName string
				if err := connRows.Scan(&toID, &toName); err == nil {
					iw.AllowedConnections = append(iw.AllowedConnections, toID)
					if toName != "" {
						iw.AllowedConnectionNames = append(iw.AllowedConnectionNames, toName)
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &iw, nil
}

// UpdateInstitutionalWork updates an existing institutional category and synchronizes connections.
func (r *Repository) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var parentID *int64
		if iw.ParentID != nil && *iw.ParentID > 0 && *iw.ParentID != iw.ID {
			parentID = iw.ParentID
		}
		const query = `
			UPDATE org.institutional_works
			SET title = $2, description = $3, icon = $4, pricing_type = $5,
			    is_active = $6, view_type = $7, slug = COALESCE(NULLIF($8, ''), slug), parent_id = $9, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			iw.ID, iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, parentID,
		)
		if err != nil {
			return fmt.Errorf("update institutional work: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}

		// Synchronize connections: delete old and insert new
		if _, err := tx.Exec(txCtx, `DELETE FROM org.institutional_work_connections WHERE from_institutional_work_id = $1;`, iw.ID); err != nil {
			return err
		}
		for _, toID := range iw.AllowedConnections {
			if toID > 0 && toID != iw.ID {
				if _, err := tx.Exec(txCtx, `
					INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
					VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
				`, iw.ID, toID); err != nil {
					return err
				}
			}
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.institutional_work.edit",
			EntityType: "institutional_work",
			EntityID:   strconv.FormatInt(iw.ID, 10),
			After:      map[string]any{"title": iw.Title, "pricing_type": iw.PricingType},
		})

		return nil
	})
}

// DeleteInstitutionalWork soft-deletes an institutional category.
func (r *Repository) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `UPDATE org.institutional_works SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.institutional_work.delete",
			EntityType: "institutional_work",
			EntityID:   strconv.FormatInt(id, 10),
		})

		return nil
	})
}

// ToggleInstitutionalWorkStatus toggles the active state of an institutional category.
func (r *Repository) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `UPDATE org.institutional_works SET is_active = NOT is_active, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}
		return nil
	})
}

// CanConnectInstitutionalWorks checks if entity fromID is permitted to connect to entity toID.
func (r *Repository) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	var allowed bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT EXISTS (
				SELECT 1 FROM org.institutional_work_connections
				WHERE from_institutional_work_id = $1 AND to_institutional_work_id = $2
			);
		`
		return tx.QueryRow(txCtx, query, fromID, toID).Scan(&allowed)
	})
	return allowed, err
}

// ListAllFlatInstitutionalWorks returns all institutional categories in a flat list with hierarchy levels and paths.
func (r *Repository) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	var all []*org.InstitutionalWork
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := seedDefaultInstitutionalWorks(txCtx, tx); err != nil {
			return err
		}

		query := `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id,
			       COALESCE(p.title->>'ar', p.title->>'en', '') AS parent_title,
			       COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id), 0) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE iw.deleted_at IS NULL
		`
		if onlyActive {
			query += " AND iw.is_active = true"
		}
		query += " ORDER BY iw.id ASC;"

		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item org.InstitutionalWork
			var pricingStr string
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.Title, &item.Description, &item.Icon,
				&pricingStr, &item.IsActive, &item.ViewType, &item.Slug, &item.ParentID,
				&item.ParentTitle, &item.BranchCount, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			item.PricingType = org.PricingType(pricingStr)
			all = append(all, &item)
		}

		// Load connections for each
		connRows, err := tx.Query(txCtx, `
			SELECT iwc.from_institutional_work_id, iwc.to_institutional_work_id,
			       COALESCE(target.title->>'ar', target.title->>'en', '')
			FROM org.institutional_work_connections iwc
			JOIN org.institutional_works target ON iwc.to_institutional_work_id = target.id
			WHERE target.deleted_at IS NULL
			ORDER BY iwc.from_institutional_work_id, target.id ASC;
		`)
		if err == nil {
			defer connRows.Close()
			connMap := make(map[int64][]int64)
			connNamesMap := make(map[int64][]string)
			for connRows.Next() {
				var fromID, toID int64
				var name string
				if err := connRows.Scan(&fromID, &toID, &name); err == nil {
					connMap[fromID] = append(connMap[fromID], toID)
					if name != "" {
						connNamesMap[fromID] = append(connNamesMap[fromID], name)
					}
				}
			}
			for _, item := range all {
				item.AllowedConnections = connMap[item.ID]
				item.AllowedConnectionNames = connNamesMap[item.ID]
			}
		}

		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return buildFlatHierarchy(all), nil
}

// ListInstitutionalWorks returns the full multi-level tree hierarchy of institutional categories.
func (r *Repository) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	flat, err := r.ListAllFlatInstitutionalWorks(ctx, onlyActive)
	if err != nil {
		return nil, err
	}

	// Build tree from flat items
	itemMap := make(map[int64]*org.InstitutionalWork)
	for _, item := range flat {
		item.Children = nil // clear for tree building
		itemMap[item.ID] = item
	}

	var rootList []*org.InstitutionalWork
	for _, item := range flat {
		if item.ParentID == nil || *item.ParentID <= 0 {
			rootList = append(rootList, item)
		} else if parent, exists := itemMap[*item.ParentID]; exists {
			parent.Children = append(parent.Children, item)
		} else {
			rootList = append(rootList, item)
		}
	}

	return rootList, nil
}

// buildFlatHierarchy computes recursive depth levels and ordered flat traversal.
func buildFlatHierarchy(items []*org.InstitutionalWork) []*org.InstitutionalWork {
	itemMap := make(map[int64]*org.InstitutionalWork)
	childrenMap := make(map[int64][]*org.InstitutionalWork)
	var roots []*org.InstitutionalWork

	for _, item := range items {
		itemMap[item.ID] = item
		if item.ParentID == nil || *item.ParentID <= 0 {
			roots = append(roots, item)
		} else {
			childrenMap[*item.ParentID] = append(childrenMap[*item.ParentID], item)
		}
	}

	var flat []*org.InstitutionalWork
	var traverse func(node *org.InstitutionalWork, level int)
	traverse = func(node *org.InstitutionalWork, level int) {
		node.Level = level
		flat = append(flat, node)
		for _, child := range childrenMap[node.ID] {
			traverse(child, level+1)
		}
	}

	for _, root := range roots {
		traverse(root, 0)
	}

	// Add any orphaned nodes that were not reached
	visited := make(map[int64]bool)
	for _, f := range flat {
		visited[f.ID] = true
	}
	for _, item := range items {
		if !visited[item.ID] {
			item.Level = 0
			flat = append(flat, item)
		}
	}

	return flat
}

// GetInstitutionalWorkBySlug retrieves an institutional work by its slug.
func (r *Repository) GetInstitutionalWorkBySlug(ctx context.Context, slug string) (*org.InstitutionalWork, error) {
	var iw org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id,
			       COALESCE(p.title->>'ar', p.title->>'en', '') AS parent_title,
			       COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id), 0) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE (iw.slug = $1 OR iw.id::text = $1) AND iw.deleted_at IS NULL
			LIMIT 1;
		`
		var pricingStr string
		err := tx.QueryRow(txCtx, query, slug).Scan(
			&iw.ID, &iw.PublicID, &iw.Title, &iw.Description, &iw.Icon, &pricingStr,
			&iw.IsActive, &iw.ViewType, &iw.Slug, &iw.ParentID, &iw.ParentTitle,
			&iw.BranchCount, &iw.CreatedAt, &iw.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("institutional_work")
			}
			return err
		}
		iw.PricingType = org.PricingType(pricingStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &iw, nil
}
