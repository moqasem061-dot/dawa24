package catalog

import (
	"context"
)

// Lifecycle methods of mockCatalogRepo.

func (m *mockCatalogRepo) ListProducts(_ context.Context, status string, limit, offset int) ([]*Product, error) {
	var list []*Product
	for _, p := range m.products {
		if status == "" || string(p.Status) == status {
			list = append(list, p)
		}
	}
	if offset >= len(list) {
		return nil, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(list) {
		end = len(list)
	}
	return list[offset:end], nil
}

func (m *mockCatalogRepo) SetProductsStatus(_ context.Context, ids []int64, status ProductStatus) (int64, error) {
	var n int64
	for _, id := range ids {
		if p, ok := m.products[id]; ok {
			p.Status = status
			n++
		}
	}
	return n, nil
}

// The mock holds no taxonomy tables; the existing GetCategoryByID and
// GetBrandByID synthesise rows on demand, so deletion only has to succeed. The
// in-use guard being tested lives in the service, above this.
func (m *mockCatalogRepo) DeleteCategory(_ context.Context, _ int64) error { return nil }

func (m *mockCatalogRepo) DeleteBrand(_ context.Context, _ int64) error { return nil }

func (m *mockCatalogRepo) CountProductsInCategory(_ context.Context, categoryID int64) (int, error) {
	count := 0
	for _, p := range m.products {
		if p.CategoryID != nil && *p.CategoryID == categoryID {
			count++
		}
	}
	return count, nil
}

func (m *mockCatalogRepo) CountProductsInBrand(_ context.Context, brandID int64) (int, error) {
	count := 0
	for _, p := range m.products {
		if p.BrandID != nil && *p.BrandID == brandID {
			count++
		}
	}
	return count, nil
}

func (m *mockCatalogRepo) ListAllMasterProductsForMatching(_ context.Context) ([]*CatalogMatchSource, error) {
	var list []*CatalogMatchSource
	for _, p := range m.products {
		list = append(list, &CatalogMatchSource{
			ID:      p.ID,
			SKU:     p.SKU,
			Barcode: p.Barcode,
			NameAr:  p.Name.Get("ar"),
			NameEn:  p.Name.Get("en"),
		})
	}
	return list, nil
}
