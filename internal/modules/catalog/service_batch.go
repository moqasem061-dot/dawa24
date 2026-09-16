package catalog

import (
	"context"
)

// ListVariantsByProducts retrieves the variants of many products in one query
// and groups them by parent product ID. The storefront catalog uses this so a
// whole page of offers costs one variant query instead of one per product.
func (s *Service) ListVariantsByProducts(ctx context.Context, productIDs []int64) (map[int64][]*ProductVariant, error) {
	out := make(map[int64][]*ProductVariant)
	if len(productIDs) == 0 {
		return out, nil
	}
	variants, err := s.repo.ListVariantsByProducts(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	for _, v := range variants {
		if v == nil || v.ProductID <= 0 {
			continue
		}
		out[v.ProductID] = append(out[v.ProductID], v)
	}
	return out, nil
}
