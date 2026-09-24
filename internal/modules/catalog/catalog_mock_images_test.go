package catalog_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
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

func (mockCatalogRepoStub) WireProductImageByLink(context.Context, string, string) error {
	return nil
}
