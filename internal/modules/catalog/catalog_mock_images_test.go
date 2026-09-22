package catalog_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func (mockCatalogRepoStub) BatchUpdateProductImage(context.Context, []int64, string, string) error {
	return nil
}

func (mockCatalogRepoStub) BatchUpdateProductImageBySKU(context.Context, []string, string, string) error {
	return nil
}

func (mockCatalogRepoStub) BatchUpdateProductImageByBarcode(context.Context, []string, string, string) error {
	return nil
}

func (mockCatalogRepoStub) ListProductsMissingImages(context.Context, int) ([]*catalog.Product, error) {
	return nil, nil
}
