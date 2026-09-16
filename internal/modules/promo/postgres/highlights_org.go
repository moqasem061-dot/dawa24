package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListHighlightSectionsByOrg returns an organization's own storefront
// merchandising/featured sections.
func (r *Repository) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	var list []*promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, title, COALESCE(description, '{"ar":"","en":""}'::jsonb),
			       COALESCE(section_type, 'about'), COALESCE(color, '#0284c7'), slug,
			       display_order, is_active, COALESCE(show_in_header, true),
			       owner_type, organization_id, created_at, updated_at
			FROM promo.highlight_sections
			WHERE owner_type = 'organization' AND organization_id = $1
			ORDER BY display_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h promo.HighlightSection
			if err := rows.Scan(
				&h.ID, &h.PublicID, &h.Title, &h.Description,
				&h.SectionType, &h.Color, &h.Slug,
				&h.DisplayOrder, &h.IsActive, &h.ShowInHeader,
				&h.OwnerType, &h.OrganizationID, &h.CreatedAt, &h.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	return list, err
}

// GetHighlightSectionByID returns a single section by ID.
func (r *Repository) GetHighlightSectionByID(ctx context.Context, id int64) (*promo.HighlightSection, error) {
	var h promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, title, COALESCE(description, '{"ar":"","en":""}'::jsonb),
			       COALESCE(section_type, 'about'), COALESCE(color, '#0284c7'), slug,
			       display_order, is_active, COALESCE(show_in_header, true),
			       owner_type, organization_id, created_at, updated_at
			FROM promo.highlight_sections
			WHERE id = $1;
		`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&h.ID, &h.PublicID, &h.Title, &h.Description,
			&h.SectionType, &h.Color, &h.Slug,
			&h.DisplayOrder, &h.IsActive, &h.ShowInHeader,
			&h.OwnerType, &h.OrganizationID, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("highlight section")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// UpdateHighlightSection modifies an existing highlight/featured section.
func (r *Repository) UpdateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.highlight_sections
			SET title = $1, description = $2, section_type = $3, color = $4,
			    slug = $5, display_order = $6, is_active = $7, show_in_header = $8, updated_at = now()
			WHERE id = $9 AND (organization_id IS NULL OR organization_id = $10);
		`
		_, err := tx.Exec(txCtx, query,
			h.Title, h.Description, h.SectionType, h.Color,
			h.Slug, h.DisplayOrder, h.IsActive, h.ShowInHeader,
			h.ID, h.OrganizationID,
		)
		return err
	})
}

// DeleteHighlightSection removes a highlight/featured section.
func (r *Repository) DeleteHighlightSection(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM promo.highlight_sections WHERE id = $1 AND (organization_id IS NULL OR organization_id = $2);`
		_, err := tx.Exec(txCtx, query, id, orgID)
		return err
	})
}

// AddHighlightItem links a product to a storefront merchandising section.
func (r *Repository) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.highlight_section_items (section_id, product_id, offer_id)
			VALUES ($1, $2, $3)
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query, item.SectionID, item.ProductID, item.OfferID).
			Scan(&item.ID)
	})
}

// ListHighlightItems returns a section's items ordered by display order.
func (r *Repository) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	var list []*promo.HighlightSectionItem
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, section_id, product_id, offer_id
			FROM promo.highlight_section_items
			WHERE section_id = $1
			ORDER BY display_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, sectionID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item promo.HighlightSectionItem
			if err := rows.Scan(&item.ID, &item.SectionID, &item.ProductID, &item.OfferID); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}
