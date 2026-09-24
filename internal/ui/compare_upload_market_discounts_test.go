package ui_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func (m *mockCompareRepoE2E) ListMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (*compare.MarketDiscountsResult, error) {
	var items []*compare.MarketDiscountRow
	for fileID, rows := range m.fileRows {
		file := m.files[fileID]
		// The board is temporary warehouses only. A supplier's own Compare Tool
		// upload is theirs, and this mock has to honour that or the e2e test
		// asserts against rows the real query would never return.
		if file == nil || !file.IsTempWarehouse {
			continue
		}
		for _, r := range rows {
			items = append(items, &compare.MarketDiscountRow{
				ID:                 r.ID,
				FileID:             file.ID,
				SupplierName:       file.SupplierName,
				ProductName:        r.RawName,
				OriginalPrice:      r.Price,
				DiscountPercent:    r.Discount,
				PriceAfterDiscount: r.PriceAfterDiscount,
				MatchedProductID:   r.MatchedProductID,
				InCatalog:          r.MatchedProductID != nil && *r.MatchedProductID > 0,
				UploadedAt:         file.CreatedAt,
			})
		}
	}
	return &compare.MarketDiscountsResult{
		Items:              items,
		TotalCount:         int64(len(items)),
		AvailableSuppliers: []string{"شركة الفتح", "شركة النصر للأدوية"},
		Page:               1,
		Limit:              24,
		TotalPages:         1,
	}, nil
}

func (m *mockCompareRepoE2E) LoadMarketOffers(
	_ context.Context, opts compare.MarketScanOptions,
) ([]compare.MarketOffer, error) {
	var out []compare.MarketOffer
	for fileID, rows := range m.fileRows {
		if opts.ExcludeFileID > 0 && fileID == opts.ExcludeFileID {
			continue
		}
		var file *compare.CompareFile
		for _, f := range m.files {
			if f != nil && f.ID == fileID {
				file = f
				break
			}
		}
		if file == nil || file.IsTempWarehouse {
			continue
		}
		if opts.ExcludeSupplierName != "" && file.SupplierName == opts.ExcludeSupplierName {
			continue
		}
		supplier := file.SupplierName
		for _, r := range rows {
			if r == nil || !r.Price.IsPositive() {
				continue
			}
			net := r.PriceAfterDiscount
			if net.IsZero() {
				net = compare.CalculatePriceAfterDiscount(r.Price, r.Discount)
			}
			out = append(out, compare.MarketOffer{
				RowID: r.ID, FileID: fileID, SupplierName: supplier,
				ProductName: r.RawName, SKU: r.SKU, ProductID: r.MatchedProductID,
				Price: r.Price, Discount: r.Discount, NetPrice: net,
			})
		}
	}
	return out, nil
}
