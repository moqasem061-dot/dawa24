package catalog

import (
	"context"
)

// BatchUpdateProductImage updates the image and image_link for multiple products by their primary IDs in a single query.
func (s *Service) BatchUpdateProductImage(ctx context.Context, productIDs []int64, imagePath string, imageLink string) error {
	if s.repo == nil || len(productIDs) == 0 {
		return nil
	}
	return s.repo.BatchUpdateProductImage(ctx, productIDs, imagePath, imageLink)
}

// BatchUpdateProductImageBySKU updates the image and image_link for products matching given SKUs.
func (s *Service) BatchUpdateProductImageBySKU(ctx context.Context, skus []string, imagePath string, imageLink string) error {
	if s.repo == nil || len(skus) == 0 {
		return nil
	}
	return s.repo.BatchUpdateProductImageBySKU(ctx, skus, imagePath, imageLink)
}

// BatchUpdateProductImageByBarcode updates the image and image_link for products matching given barcodes.
func (s *Service) BatchUpdateProductImageByBarcode(ctx context.Context, barcodes []string, imagePath string, imageLink string) error {
	if s.repo == nil || len(barcodes) == 0 {
		return nil
	}
	return s.repo.BatchUpdateProductImageByBarcode(ctx, barcodes, imagePath, imageLink)
}

// ListProductsMissingImages retrieves products that have an external image link but no locally stored image on volume.
func (s *Service) ListProductsMissingImages(ctx context.Context, limit int) ([]*Product, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.ListProductsMissingImages(ctx, limit)
}

// WireProductImageByLink updates products with a given image_link or remote image URL to a local volume path.
func (s *Service) WireProductImageByLink(ctx context.Context, remoteURL, localPath string) error {
	if s.repo == nil || remoteURL == "" || localPath == "" {
		return nil
	}
	return s.repo.WireProductImageByLink(ctx, remoteURL, localPath)
}
