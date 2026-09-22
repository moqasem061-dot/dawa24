package http_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func (r stubRepo) BatchUpdateProductImage(ctx context.Context, productIDs []int64, imagePath, imageLink string) error {
	return nil
}

func (r stubRepo) BatchUpdateProductImageBySKU(ctx context.Context, skus []string, imagePath, imageLink string) error {
	return nil
}

func (r stubRepo) BatchUpdateProductImageByBarcode(ctx context.Context, barcodes []string, imagePath, imageLink string) error {
	return nil
}

func (r stubRepo) ListProductsMissingImages(ctx context.Context, limit int) ([]*catalog.Product, error) {
	return nil, nil
}

func (happyRepo) BatchUpdateProductImage(ctx context.Context, productIDs []int64, imagePath, imageLink string) error {
	return nil
}

func (happyRepo) BatchUpdateProductImageBySKU(ctx context.Context, skus []string, imagePath, imageLink string) error {
	return nil
}

func (happyRepo) BatchUpdateProductImageByBarcode(ctx context.Context, barcodes []string, imagePath, imageLink string) error {
	return nil
}

func (happyRepo) ListProductsMissingImages(ctx context.Context, limit int) ([]*catalog.Product, error) {
	return nil, nil
}
