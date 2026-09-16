package compare

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RunMultiSupplierComparison executes multi-supplier analysis across 1 to 10 selected files (Plan V5 Phase 2 §2.5).
func (s *Service) RunMultiSupplierComparison(ctx context.Context, fileIDs []int64) (*ComparisonResultSet, error) {
	if len(fileIDs) < 1 {
		return nil, apperr.Validation("compare.no_suppliers", "يرجى اختيار مورد واحد على الأقل للمقارنة.", nil)
	}
	if len(fileIDs) > 10 {
		return nil, apperr.Validation("compare.max_suppliers", "الحد الأقصى للمقارنة هو 10 موردين في المرة الواحدة.", nil)
	}

	// Fetch all rows for the selected files
	var files []*CompareFile
	for _, id := range fileIDs {
		f, err := s.repo.GetFileByID(ctx, id)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	var resultRows []*ProductComparisonRow
	bySKU := make(map[string]*ProductComparisonRow)
	byNorm := make(map[string]*ProductComparisonRow)
	byCore := make(map[string]*ProductComparisonRow)
	bySorted := make(map[string]*ProductComparisonRow)

	// keyOfRow remembers what each comparison row IS, so a SKU collision cannot
	// merge two products. See the guard below.
	keyOfRow := make(map[*ProductComparisonRow]string)

	supplierBestCounts := make(map[string]int)
	var suppliersList []string

	for _, f := range files {
		suppliersList = append(suppliersList, f.SupplierName)
		rows, err := s.repo.ListFileRows(ctx, f.ID, 100000, 0)
		if err != nil {
			return nil, err
		}

		for _, r := range rows {
			normText := normalizeProductText(r.RawName)
			if normText == "" {
				normText = r.NormalizedName
			}
			coreKey := getCoreDrugMatchKey(r.RawName)
			sortedKey := getSortedWordsKey(r.RawName)
			cleanSKU := strings.ToLower(strings.TrimSpace(r.SKU))

			var compRow *ProductComparisonRow

			// 1. Match across vendor files by clean SKU — but never against a
			//    row that is demonstrably a different product.
			//
			// These are two SUPPLIERS' internal item numbers, not a shared
			// catalogue code. Supplier A's "951" and supplier B's "951" are the
			// same string by coincidence far more often than by agreement, and
			// this index merged them with no check on the name at all: one
			// comparison line, two unrelated medicines, and a "best price" that
			// was simply the cheaper of the two drugs.
			//
			// The guard is deliberately narrow. A code hit is still accepted
			// when either row's name could not be read — that is the case the
			// code index exists for — and refused only when both names read
			// clearly and say different products.
			if cleanSKU != "" {
				if candidate := bySKU[cleanSKU]; candidate != nil {
					existing := keyOfRow[candidate]
					if existing == "" || coreKey == "" || existing == coreKey {
						compRow = candidate
					}
				}
			}
			// 2. Match across vendor files by exact normalized Arabic/English name
			if compRow == nil && normText != "" {
				compRow = byNorm[normText]
			}
			// 3. Match across vendor files by core drug phonetic/noise-free key
			if compRow == nil && coreKey != "" {
				compRow = byCore[coreKey]
			}
			// 4. Match across vendor files by bag-of-words sorted key
			if compRow == nil && sortedKey != "" {
				compRow = bySorted[sortedKey]
			}

			netPrice := r.PriceAfterDiscount
			if netPrice.IsZero() && r.Price.IsPositive() {
				netPrice = CalculatePriceAfterDiscount(r.Price, r.Discount)
			}

			offer := SupplierOffer{
				SupplierName:       f.SupplierName,
				Price:              r.Price,
				Discount:           r.Discount,
				PriceAfterDiscount: netPrice,
			}

			if compRow == nil {
				compRow = &ProductComparisonRow{
					ProductName:  r.RawName,
					SKU:          r.SKU,
					Offers:       make(map[string]SupplierOffer),
					BestPrice:    r.Price,
					BestDiscount: r.Discount,
					BestNetPrice: netPrice,
					BestSupplier: f.SupplierName,
				}
				resultRows = append(resultRows, compRow)
				keyOfRow[compRow] = coreKey

				// Register in all lookup indices
				if cleanSKU != "" {
					bySKU[cleanSKU] = compRow
				}
				if normText != "" {
					byNorm[normText] = compRow
				}
				if coreKey != "" {
					byCore[coreKey] = compRow
				}
				if sortedKey != "" {
					bySorted[sortedKey] = compRow
				}
			}

			compRow.Offers[f.SupplierName] = offer

			// Update best offer if this supplier has a better (lower) net price or higher discount
			if netPrice.Minor() < compRow.BestNetPrice.Minor() || (netPrice.Minor() == compRow.BestNetPrice.Minor() && r.Discount > compRow.BestDiscount) {
				compRow.BestNetPrice = netPrice
				compRow.BestPrice = r.Price
				compRow.BestDiscount = r.Discount
				compRow.BestSupplier = f.SupplierName
			}
		}
	}

	totalDiscountSum := 0.0
	discountCount := 0

	for _, row := range resultRows {
		row.TotalSuppliers = len(row.Offers)
		supplierBestCounts[row.BestSupplier]++

		// Set 3-way catalog status
		if row.MatchedProductID != nil && *row.MatchedProductID > 0 {
			row.InCatalog = true
			row.CatalogStatus = StatusCatalogAndSuppliers
		} else {
			row.InCatalog = false
			row.CatalogStatus = StatusSupplierCustom
		}

		// Calculate missing suppliers from selected comparison files
		for _, sup := range suppliersList {
			if _, hasOffer := row.Offers[sup]; !hasOffer {
				row.MissingSuppliers = append(row.MissingSuppliers, sup)
			}
		}

		// Determine availability status
		if len(row.MissingSuppliers) == 0 {
			row.AvailabilityStatus = "all_suppliers"
		} else if len(row.Offers) > 1 {
			row.AvailabilityStatus = "multi_supplier"
		} else {
			row.AvailabilityStatus = "single_exclusive"
		}

		for _, off := range row.Offers {
			if off.Discount > 0 {
				totalDiscountSum += off.Discount
				discountCount++
			}
		}
	}

	// Sort results alphabetically by product name by default
	sort.Slice(resultRows, func(i, j int) bool {
		return resultRows[i].ProductName < resultRows[j].ProductName
	})

	avgDiscount := 0.0
	if discountCount > 0 {
		avgDiscount = math.Round((totalDiscountSum/float64(discountCount))*100.0) / 100.0
	}

	summary := ComparisonSummary{
		TotalProducts:        len(resultRows),
		TotalSuppliers:       len(files),
		SuppliersList:        suppliersList,
		AverageDiscount:      avgDiscount,
		BestOffersBySupplier: supplierBestCounts,
	}

	return &ComparisonResultSet{
		Rows:    resultRows,
		Summary: summary,
	}, nil
}

// RunSupplierVsSupplier executes head-to-head comparison between two suppliers (Plan V5 Phase 2 §2.5.1).
func (s *Service) RunSupplierVsSupplier(ctx context.Context, sourceFileID, targetFileID int64) ([]*HeadToHeadItem, *HeadToHeadStats, error) {
	detailed, err := s.RunSupplierVsSupplierDetailed(ctx, HeadToHeadFilter{
		SourceFileID: sourceFileID,
		TargetFileID: targetFileID,
	})
	if err != nil {
		return nil, nil, err
	}

	var items []*HeadToHeadItem
	for _, r := range detailed.Rows {
		items = append(items, &HeadToHeadItem{
			ProductName:    r.ProductName,
			SKU:            r.SKU,
			SourcePrice:    r.Price,
			SourceDiscount: r.YourDiscount,
			SourceNet:      r.YourNetPrice,
			TargetPrice:    r.Price,
			TargetDiscount: r.CompetitorDiscount,
			TargetNet:      r.CompetitorNetPrice,
			IsBetter:       r.Outcome == OutcomeYourBetter || r.Outcome == OutcomeEqual,
			PriceDiff:      r.SavingsDifference,
		})
	}

	stats := &HeadToHeadStats{
		SharedCount:  detailed.TotalShared,
		BetterCount:  detailed.YourBetterCount + detailed.EqualCount,
		SourceTotal:  detailed.SourceTotal,
		TargetTotal:  detailed.TargetTotal,
		QualityScore: detailed.WinRatePercent,
		TotalSavings: detailed.TotalSavings,
	}

	return items, stats, nil
}
