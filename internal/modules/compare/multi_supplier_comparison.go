package compare

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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
	byProductID := make(map[int64]*ProductComparisonRow)
	byBarcode := make(map[string]*ProductComparisonRow)
	bySKU := make(map[string]*ProductComparisonRow)
	byNorm := make(map[string]*ProductComparisonRow)
	byCore := make(map[string]*ProductComparisonRow)
	bySorted := make(map[string]*ProductComparisonRow)
	byBaseCore := make(map[string][]*multiCompBaseEntry)

	// keyOfRow remembers what each comparison row IS, so a SKU collision cannot
	// merge two products. See the guard below.
	keyOfRow := make(map[*ProductComparisonRow]string)

	supplierBestCounts := make(map[string]int)
	var suppliersList []string

	type fileWithRows struct {
		file *CompareFile
		rows []*CompareFileRow
	}
	var loadedFiles []fileWithRows
	var productIDs []int64
	seenProd := make(map[int64]bool)

	for _, f := range files {
		suppliersList = append(suppliersList, f.SupplierName)
		rows, err := s.repo.ListFileRows(ctx, f.ID, 100000, 0)
		if err != nil {
			return nil, err
		}
		loadedFiles = append(loadedFiles, fileWithRows{file: f, rows: rows})
		for _, r := range rows {
			if r != nil && r.MatchedProductID != nil && *r.MatchedProductID > 0 && !seenProd[*r.MatchedProductID] {
				productIDs = append(productIDs, *r.MatchedProductID)
				seenProd[*r.MatchedProductID] = true
			}
		}
	}

	var rels []productmatch.PackRelationship
	if len(productIDs) > 0 {
		rels, _ = s.repo.LoadPackRelationships(ctx, productIDs)
	}

	for _, fw := range loadedFiles {
		f := fw.file
		rows := fw.rows
		for _, r := range rows {
			if r == nil || !IsValidProductRow(r.RawName, r.SKU, r.Price, r.Discount) {
				continue
			}
			normText := normalizeProductText(r.RawName)
			if normText == "" {
				normText = r.NormalizedName
			}
			coreKey := getCoreDrugMatchKey(r.RawName)
			sortedKey := getSortedWordsKey(r.RawName)
			cleanSKU := strings.ToLower(strings.TrimSpace(r.SKU))

			var compRow *ProductComparisonRow

			// 1. Match across vendor files by master catalog product ID (exact 100% catalog match)
			if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
				if candidate := byProductID[*r.MatchedProductID]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}

			// 2. Match across vendor files by clean barcode (8 to 14 digits)
			if compRow == nil && cleanSKU != "" && isBarcode(cleanSKU) {
				if candidate := byBarcode[cleanSKU]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}

			// 3. Match across vendor files by clean SKU
			if compRow == nil && cleanSKU != "" {
				if candidate := bySKU[cleanSKU]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}
			// 4. Match across vendor files by exact normalized Arabic/English name
			if compRow == nil && normText != "" {
				if candidate := byNorm[normText]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}
			// 5. Match across vendor files by core drug phonetic/noise-free key
			if compRow == nil && coreKey != "" {
				if candidate := byCore[coreKey]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}
			// 6. Match across vendor files by bag-of-words sorted key
			if compRow == nil && sortedKey != "" {
				if candidate := bySorted[sortedKey]; candidate != nil {
					if ok, _, _ := canGroupWithProductRow(r, candidate, rels); ok {
						compRow = candidate
					}
				}
			}
			// 7. Match across vendor files by Base Product Key with Pack Compatibility
			if compRow == nil {
				baseKey, pCount, hasCount := productmatch.ProductKeyParts(r.RawName)
				if baseKey != "" {
					candidates := byBaseCore[baseKey]
					for _, c := range candidates {
						if ok, _, _ := canGroupWithProductRow(r, c.compRow, rels); ok {
							if hasCount && c.hasCount {
								if math.Abs(pCount-c.packCount) < 0.01 {
									compRow = c.compRow
									break
								}
							} else if isPriceCompatible(r.Price, c.compRow.BestPrice) {
								compRow = c.compRow
								break
							}
						}
					}
				}
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

			if compRow != nil {
				_, groupBasis, notComparable := canGroupWithProductRow(r, compRow, rels)
				if compRow.GroupBasis == "" {
					compRow.GroupBasis = groupBasis
				}
				if notComparable {
					compRow.NotComparable = true
					compRow.GroupBasis = "not_comparable"
				}

				rFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: r.RawName, SKU: r.SKU}, nil)
				compFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: compRow.ProductName, SKU: compRow.SKU}, nil)
				convertedNet, convertible := convertEquivalentPrice(netPrice, compFacts, rFacts, rels)
				if !convertible {
					compRow.NotComparable = true
					compRow.GroupBasis = "not_comparable"
				} else if !compRow.NotComparable {
					offer.PriceAfterDiscount = convertedNet
					if convertedNet.Minor() < compRow.BestNetPrice.Minor() || (convertedNet.Minor() == compRow.BestNetPrice.Minor() && r.Discount > compRow.BestDiscount) {
						compRow.BestNetPrice = convertedNet
						compRow.BestPrice = r.Price
						compRow.BestDiscount = r.Discount
						compRow.BestSupplier = f.SupplierName
					}
				}
			}

			if compRow == nil {
				groupBasis := "source_equivalence"
				if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
					groupBasis = "canonical_pack"
				}
				compRow = &ProductComparisonRow{
					ProductName:      r.RawName,
					SKU:              r.SKU,
					MatchedProductID: r.MatchedProductID,
					Offers:           make(map[string]SupplierOffer),
					BestPrice:        r.Price,
					BestDiscount:     r.Discount,
					BestNetPrice:     netPrice,
					BestSupplier:     f.SupplierName,
					GroupBasis:       groupBasis,
				}
				resultRows = append(resultRows, compRow)
				keyOfRow[compRow] = coreKey

				// Register in all lookup indices
				if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
					byProductID[*r.MatchedProductID] = compRow
				}
				if cleanSKU != "" {
					if isBarcode(cleanSKU) {
						byBarcode[cleanSKU] = compRow
					}
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
				baseKey, pCount, hasCount := productmatch.ProductKeyParts(r.RawName)
				if baseKey != "" {
					byBaseCore[baseKey] = append(byBaseCore[baseKey], &multiCompBaseEntry{
						compRow:   compRow,
						packCount: pCount,
						hasCount:  hasCount,
					})
				}
			} else {
				if compRow.MatchedProductID == nil && r.MatchedProductID != nil && *r.MatchedProductID > 0 {
					compRow.MatchedProductID = r.MatchedProductID
					byProductID[*r.MatchedProductID] = compRow
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

	var totalPotentialSaving money.Amount
	for _, row := range resultRows {
		if row.NotComparable || len(row.Offers) <= 1 {
			continue
		}
		var minNet, maxNet money.Amount
		first := true
		for _, off := range row.Offers {
			if first {
				minNet = off.PriceAfterDiscount
				maxNet = off.PriceAfterDiscount
				first = false
			} else {
				if off.PriceAfterDiscount.Minor() < minNet.Minor() {
					minNet = off.PriceAfterDiscount
				}
				if off.PriceAfterDiscount.Minor() > maxNet.Minor() {
					maxNet = off.PriceAfterDiscount
				}
			}
		}
		saving, _ := maxNet.Sub(minNet)
		totalPotentialSaving, _ = totalPotentialSaving.Add(saving)
	}

	summary := ComparisonSummary{
		TotalProducts:        len(resultRows),
		TotalSuppliers:       len(files),
		SuppliersList:        suppliersList,
		AverageDiscount:      avgDiscount,
		BestOffersBySupplier: supplierBestCounts,
		TotalPotentialSaving: totalPotentialSaving,
	}

	return &ComparisonResultSet{
		Rows:    resultRows,
		Summary: summary,
	}, nil
}

