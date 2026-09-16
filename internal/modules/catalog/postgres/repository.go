package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements catalog.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL catalog repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateProduct inserts a new product for the active organization.
func (r *Repository) CreateProduct(ctx context.Context, p *catalog.Product) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.OrganizationID <= 0 {
			var firstOrgID int64
			err := tx.QueryRow(txCtx, `SELECT id FROM org.organizations WHERE status = 'approved' OR type = 'vendor' ORDER BY id ASC LIMIT 1`).Scan(&firstOrgID)
			if err != nil || firstOrgID <= 0 {
				_ = tx.QueryRow(txCtx, `SELECT id FROM org.organizations ORDER BY id ASC LIMIT 1`).Scan(&firstOrgID)
			}
			if firstOrgID > 0 {
				p.OrganizationID = firstOrgID
			} else {
				err = tx.QueryRow(txCtx, `
					INSERT INTO org.organizations (name, legal_name, trade_name, type, status)
					VALUES ('{"ar":"دوا 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, '{"ar":"دوا 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, '{"ar":"دوا 24","en":"Dawa24"}'::jsonb, 'vendor', 'approved')
					RETURNING id
				`).Scan(&firstOrgID)
				if err == nil {
					p.OrganizationID = firstOrgID
				}
			}
		}

		if p.InstitutionalWorkIDs == nil {
			p.InstitutionalWorkIDs = []int64{}
		}

		query := `
			INSERT INTO catalog.products (
				organization_id, category_id, brand_id, branch_id, name, description,
				sku, barcode, price, discount, old_price, image, image_link, status,
				is_featured, dosage_form, scientific_name, pharmacology, active,
				concentration, unit, manufacturing_companies, institutional_work_ids
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, COALESCE($23, '{}'::bigint[])
			) RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
			p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
			string(p.Status), p.IsFeatured, p.DosageForm, p.ScientificName,
			p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
			p.InstitutionalWorkIDs,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)

		if err != nil {
			return fmt.Errorf("catalog postgres: create product: %w", err)
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &p.OrganizationID,
			Action:         "catalog.product.create",
			EntityType:     "product",
			EntityID:       strconv.FormatInt(p.ID, 10),
			After:          map[string]any{"sku": p.SKU, "name": p.Name, "price": p.Price},
		})
		return nil
	})
}

// GetProductByID retrieves a product by its primary key.
func (r *Repository) GetProductByID(ctx context.Context, id int64) (*catalog.Product, error) {
	var p catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
			&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
			&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
			&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
			&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product")
			}
			return fmt.Errorf("catalog postgres: get product: %w", err)
		}
		p.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProduct updates product attributes.
func (r *Repository) UpdateProduct(ctx context.Context, p *catalog.Product) error {
	if p.InstitutionalWorkIDs == nil {
		p.InstitutionalWorkIDs = []int64{}
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET category_id = $2, brand_id = $3, branch_id = $4, name = $5,
			    description = $6, sku = $7, barcode = $8, price = $9, discount = $10,
			    old_price = $11, image = $12, image_link = $13, status = $14,
			    is_featured = $15, dosage_form = $16, scientific_name = $17,
			    pharmacology = $18, active = $19, concentration = $20, unit = $21,
			    manufacturing_companies = $22, institutional_work_ids = COALESCE($23, '{}'::bigint[]), updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query,
			p.ID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
			p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
			string(p.Status), p.IsFeatured, p.DosageForm, p.ScientificName,
			p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
			p.InstitutionalWorkIDs,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: update product: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product")
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &p.OrganizationID,
			Action:         "catalog.product.update",
			EntityType:     "product",
			EntityID:       strconv.FormatInt(p.ID, 10),
			After:          map[string]any{"sku": p.SKU, "name": p.Name, "price": p.Price, "status": p.Status},
		})
		return nil
	})
}

// DeleteProduct soft-deletes a product, its variants, and associated warehouse stocks.
func (r *Repository) DeleteProduct(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.products SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		res, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product")
		}

		// Cascade soft-delete to child variants and stocks
		_, _ = tx.Exec(txCtx, `UPDATE catalog.product_variants SET deleted_at = now() WHERE product_id = $1 AND deleted_at IS NULL;`, id)
		_, _ = tx.Exec(txCtx, `UPDATE inventory.stocks SET deleted_at = now() WHERE product_id = $1 AND deleted_at IS NULL;`, id)

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "catalog.product.delete",
			EntityType: "product",
			EntityID:   strconv.FormatInt(id, 10),
		})
		return nil
	})
}

// GetProductBySKU retrieves a master product by its exact SKU (or barcode).
func (r *Repository) GetProductBySKU(ctx context.Context, sku string) (*catalog.Product, error) {
	var p catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE (sku = $1 OR barcode = $1) AND deleted_at IS NULL
			ORDER BY id ASC LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, sku).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
			&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
			&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
			&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
			&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product")
			}
			return fmt.Errorf("catalog postgres: get product by sku: %w", err)
		}
		p.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProductImageBySKU sets the downloaded image path and source URL for a product matching the SKU.
func (r *Repository) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath string, imageLink string) (*catalog.Product, error) {
	prod, err := r.GetProductBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	prod.Image = imagePath
	if imageLink != "" {
		prod.ImageLink = imageLink
	}
	if err := r.UpdateProduct(ctx, prod); err != nil {
		return nil, err
	}
	return prod, nil
}

// GetProductByBarcode retrieves a master product by its exact barcode.
func (r *Repository) GetProductByBarcode(ctx context.Context, barcode string) (*catalog.Product, error) {
	var p catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE barcode = $1 AND deleted_at IS NULL
			ORDER BY id ASC LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, barcode).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
			&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
			&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
			&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
			&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product")
			}
			return fmt.Errorf("catalog postgres: get product by barcode: %w", err)
		}
		p.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProductImageByID sets the downloaded image path and source URL for a product by its ID.
func (r *Repository) UpdateProductImageByID(ctx context.Context, id int64, imagePath string, imageLink string) (*catalog.Product, error) {
	prod, err := r.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	prod.Image = imagePath
	if imageLink != "" {
		prod.ImageLink = imageLink
	}
	if err := r.UpdateProduct(ctx, prod); err != nil {
		return nil, err
	}
	return prod, nil
}
