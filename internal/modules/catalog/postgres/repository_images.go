package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// BatchUpdateProductImage updates the image and image_link for multiple products by their primary IDs in a single query.
func (r *Repository) BatchUpdateProductImage(ctx context.Context, productIDs []int64, imagePath string, imageLink string) error {
	if len(productIDs) == 0 || imagePath == "" {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET image = $1,
			    image_link = CASE WHEN $2 <> '' THEN $2 ELSE image_link END,
			    updated_at = now()
			WHERE id = ANY($3) AND deleted_at IS NULL;
		`
		_, err := tx.Exec(txCtx, query, imagePath, imageLink, productIDs)
		if err != nil {
			return fmt.Errorf("catalog postgres: batch update product images: %w", err)
		}
		return nil
	})
}

// BatchUpdateProductImageBySKU updates the image and image_link for products matching given SKUs.
func (r *Repository) BatchUpdateProductImageBySKU(ctx context.Context, skus []string, imagePath string, imageLink string) error {
	if len(skus) == 0 || imagePath == "" {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET image = $1,
			    image_link = CASE WHEN $2 <> '' THEN $2 ELSE image_link END,
			    updated_at = now()
			WHERE sku = ANY($3) AND deleted_at IS NULL;
		`
		_, err := tx.Exec(txCtx, query, imagePath, imageLink, skus)
		if err != nil {
			return fmt.Errorf("catalog postgres: batch update product images by sku: %w", err)
		}
		return nil
	})
}

// BatchUpdateProductImageByBarcode updates the image and image_link for products matching given barcodes.
func (r *Repository) BatchUpdateProductImageByBarcode(ctx context.Context, barcodes []string, imagePath string, imageLink string) error {
	if len(barcodes) == 0 || imagePath == "" {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET image = $1,
			    image_link = CASE WHEN $2 <> '' THEN $2 ELSE image_link END,
			    updated_at = now()
			WHERE barcode = ANY($3) AND deleted_at IS NULL;
		`
		_, err := tx.Exec(txCtx, query, imagePath, imageLink, barcodes)
		if err != nil {
			return fmt.Errorf("catalog postgres: batch update product images by barcode: %w", err)
		}
		return nil
	})
}

// ListProductsMissingImages retrieves products that have an external image link but no locally stored image on volume.
func (r *Repository) ListProductsMissingImages(ctx context.Context, limit int) ([]*catalog.Product, error) {
	if limit <= 0 {
		limit = 1000
	}
	var products []*catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, sku, barcode, image_link
			FROM catalog.products
			WHERE deleted_at IS NULL
			  AND (image IS NULL OR image = '')
			  AND (image_link IS NOT NULL AND image_link <> '' AND (image_link LIKE 'http%' OR image_link LIKE '//%'))
			ORDER BY id ASC
			LIMIT $1;
		`
		rows, err := tx.Query(txCtx, query, limit)
		if err != nil {
			return fmt.Errorf("catalog postgres: list products missing images: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.Product
			if err := rows.Scan(&p.ID, &p.SKU, &p.Barcode, &p.ImageLink); err != nil {
				return fmt.Errorf("catalog postgres: scan product missing image: %w", err)
			}
			products = append(products, &p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return products, nil
}
