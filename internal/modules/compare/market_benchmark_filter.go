package compare

import (
	"math"
	"sort"
	"strings"
)

// applyBenchmarkFilter narrows and orders benchmark rows according to the filter.
func applyBenchmarkFilter(rows []*BenchmarkRow, f BenchmarkFilter) []*BenchmarkRow {
	qRaw := strings.ToLower(strings.TrimSpace(f.Query))
	qNorm := normalizeProductText(f.Query)
	out := make([]*BenchmarkRow, 0, len(rows))

	minPriceMinor := priceMinor(f.MinPrice)
	maxPriceMinor := priceMinor(f.MaxPrice)
	if minPriceMinor != nil && maxPriceMinor != nil && *minPriceMinor > *maxPriceMinor {
		minPriceMinor, maxPriceMinor = maxPriceMinor, minPriceMinor
	}

	minDiscount := f.MinDiscount
	maxDiscount := f.MaxDiscount
	if minDiscount != nil && maxDiscount != nil && *minDiscount > *maxDiscount {
		minDiscount, maxDiscount = maxDiscount, minDiscount
	}

	for _, row := range rows {
		if row == nil {
			continue
		}
		if qRaw != "" {
			nameLower := strings.ToLower(row.ProductName)
			skuLower := strings.ToLower(row.SKU)
			nameNorm := normalizeProductText(row.ProductName)
			if !strings.Contains(nameLower, qRaw) &&
				!strings.Contains(skuLower, qRaw) &&
				(qNorm == "" || !strings.Contains(nameNorm, qNorm)) {
				continue
			}
		}
		if minPriceMinor != nil && row.YourNet.Minor() < *minPriceMinor {
			continue
		}
		if maxPriceMinor != nil && row.YourNet.Minor() > *maxPriceMinor {
			continue
		}
		if minDiscount != nil && row.YourDiscount < *minDiscount {
			continue
		}
		if maxDiscount != nil && row.YourDiscount > *maxDiscount {
			continue
		}
		switch f.Tab {
		case "", "all":
		case BenchHigher, BenchEqual, BenchBetter, BenchExclusive:
			if row.Classification != f.Tab {
				continue
			}
		}
		out = append(out, row)
	}

	sortBenchmarkRows(out, f.Sort)
	return out
}

func priceMinor(val *float64) *int64 {
	if val == nil {
		return nil
	}
	m := int64(math.Round(*val * 100))
	return &m
}

func sortBenchmarkRows(rows []*BenchmarkRow, sortKey string) {
	switch sortKey {
	case "price_asc", "price":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].YourNet.Minor() < rows[j].YourNet.Minor()
		})
	case "price_desc":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].YourNet.Minor() > rows[j].YourNet.Minor()
		})
	case "discount_desc", "discount":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].YourDiscount > rows[j].YourDiscount
		})
	case "discount_asc":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].YourDiscount < rows[j].YourDiscount
		})
	case "better_offers_desc":
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].BetterOffers != rows[j].BetterOffers {
				return rows[i].BetterOffers > rows[j].BetterOffers
			}
			return rows[i].PriceGap.Minor() > rows[j].PriceGap.Minor()
		})
	case "suppliers_desc":
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].MarketSuppliers != rows[j].MarketSuppliers {
				return rows[i].MarketSuppliers > rows[j].MarketSuppliers
			}
			return rows[i].ProductName < rows[j].ProductName
		})
	case "name_asc":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].ProductName < rows[j].ProductName
		})
	case "gap_asc":
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].PriceGap.Minor() != rows[j].PriceGap.Minor() {
				return rows[i].PriceGap.Minor() < rows[j].PriceGap.Minor()
			}
			return rows[i].YourNet.Minor() < rows[j].YourNet.Minor()
		})
	default: // "gap_desc" or empty
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].PriceGap.Minor() != rows[j].PriceGap.Minor() {
				return rows[i].PriceGap.Minor() > rows[j].PriceGap.Minor()
			}
			return rows[i].YourNet.Minor() > rows[j].YourNet.Minor()
		})
	}
}
