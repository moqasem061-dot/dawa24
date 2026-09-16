package compare_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// LoadMarketOffers is the analytical read the two market screens use. The mock
// applies the same rule the real query does: temporary warehouses, plus the
// caller's own organisation's uploads. A compare-tool upload belonging to
// another tenant is never in "the market", whatever its visibility column says.
func (m *mockCompareRepo) LoadMarketOffers(
	_ context.Context, opts compare.MarketScanOptions,
) ([]compare.MarketOffer, error) {
	var out []compare.MarketOffer
	for _, f := range m.files {
		if f == nil || f.DeletedAt != nil || f.Status != compare.FileReady {
			continue
		}
		if f.IsTempWarehouse {
			continue
		}
		if opts.ExcludeFileID > 0 && f.ID == opts.ExcludeFileID {
			continue
		}
		if opts.ExcludeSupplierName != "" && f.SupplierName == opts.ExcludeSupplierName {
			continue
		}
		if opts.OrganizationID != nil && f.OrganizationID != nil &&
			*f.OrganizationID != *opts.OrganizationID {
			continue
		}
		for _, r := range m.fileRows[f.ID] {
			if r == nil || !r.Price.IsPositive() {
				continue
			}
			net := r.PriceAfterDiscount
			if net.IsZero() {
				net = compare.CalculatePriceAfterDiscount(r.Price, r.Discount)
			}
			out = append(out, compare.MarketOffer{
				RowID: r.ID, FileID: f.ID, SupplierName: f.SupplierName,
				ProductName: r.RawName, SKU: r.SKU, ProductID: r.MatchedProductID,
				Price: r.Price, Discount: r.Discount, NetPrice: net,
			})
		}
	}
	return out, nil
}
