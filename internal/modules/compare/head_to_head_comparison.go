package compare

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// RunSupplierVsSupplierDetailed executes full head-to-head comparison with custom filtering, matching, and metrics.
func (s *Service) RunSupplierVsSupplierDetailed(ctx context.Context, filter HeadToHeadFilter) (*HeadToHeadComparisonResult, error) {
	if filter.SourceFileID <= 0 || filter.TargetFileID <= 0 {
		return nil, apperr.Validation("compare.invalid_files", "يرجى تحديد المورد الأساسي والمورد المقارن به للمتابعة.", nil)
	}

	sourceFile, err := s.repo.GetFileByID(ctx, filter.SourceFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source file: %w", err)
	}
	targetFile, err := s.repo.GetFileByID(ctx, filter.TargetFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target file: %w", err)
	}

	sourceRows, err := s.repo.ListFileRows(ctx, filter.SourceFileID, 100000, 0)
	if err != nil {
		return nil, err
	}
	targetRows, err := s.repo.ListFileRows(ctx, filter.TargetFileID, 100000, 0)
	if err != nil {
		return nil, err
	}

	// Index target rows
	targetBySKU := make(map[string]*CompareFileRow)
	targetByBarcode := make(map[string]*CompareFileRow)
	targetByName := make(map[string]*CompareFileRow)
	targetByCore := make(map[string]*CompareFileRow)
	targetBySorted := make(map[string]*CompareFileRow)
	targetByProductID := make(map[int64]*CompareFileRow)
	targetByBaseKey := make(map[string][]*targetRowEntry)

	// Prepare candidates for fuzzy matching fallback
	targetByID := make(map[int64]*CompareFileRow, len(targetRows))
	masters := make([]productmatch.MasterProduct, 0, len(targetRows))

	for i, tr := range targetRows {
		if tr.MatchedProductID != nil && *tr.MatchedProductID > 0 {
			targetByProductID[*tr.MatchedProductID] = tr
		}
		cleanSKU := strings.ToLower(strings.TrimSpace(tr.SKU))
		if cleanSKU != "" {
			if isBarcode(cleanSKU) {
				targetByBarcode[cleanSKU] = tr
			}
			targetBySKU[cleanSKU] = tr
		}
		norm := normalizeProductText(tr.RawName)
		if norm != "" {
			targetByName[norm] = tr
		}
		coreKey := getCoreDrugMatchKey(tr.RawName)
		if coreKey != "" {
			targetByCore[coreKey] = tr
		}
		sortedKey := getSortedWordsKey(tr.RawName)
		if sortedKey != "" {
			targetBySorted[sortedKey] = tr
		}
		baseKey, pCount, hasCount := productmatch.ProductKeyParts(tr.RawName)
		if baseKey != "" {
			targetByBaseKey[baseKey] = append(targetByBaseKey[baseKey], &targetRowEntry{
				row:       tr,
				packCount: pCount,
				hasCount:  hasCount,
			})
		}

		tID := tr.ID
		if tID <= 0 {
			tID = int64(i + 1)
		}
		targetByID[tID] = tr
		masters = append(masters, productmatch.MasterProduct{
			ID:      tID,
			NameAR:  tr.RawName,
			NameEN:  tr.RawName,
			SKU:     tr.SKU,
			Barcode: tr.SKU,
		})
	}

	var fuzzyIndex *productmatch.Index
	if len(masters) > 0 {
		fuzzyIndex = productmatch.NewIndex(masters)
	}
	shortKeys := newShortBrandPairs(sourceRows, targetRows)

	var allSharedRows []*HeadToHeadRow
	yourBetterCount := 0
	equalCount := 0
	competitorBetterCount := 0
	var sourceTotal money.Amount
	var targetTotal money.Amount

	qLower := strings.ToLower(strings.TrimSpace(filter.Query))

	for _, sr := range sourceRows {
		var tr *CompareFileRow
		// 1. Matched master catalog ID
		if sr.MatchedProductID != nil && *sr.MatchedProductID > 0 {
			tr = targetByProductID[*sr.MatchedProductID]
		}
		// 2. Barcode
		if tr == nil {
			cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))
			if cleanSKU != "" && isBarcode(cleanSKU) {
				tr = targetByBarcode[cleanSKU]
			}
		}
		// 3. SKU
		if tr == nil {
			cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))
			if cleanSKU != "" {
				tr = targetBySKU[cleanSKU]
			}
		}
		// 4. Normalized name
		if tr == nil {
			norm := normalizeProductText(sr.RawName)
			tr = targetByName[norm]
		}
		// 5. Exact ProductKey
		if tr == nil {
			coreKey := getCoreDrugMatchKey(sr.RawName)
			tr = targetByCore[coreKey]
		}
		// 6. Bag of words
		if tr == nil {
			sortedKey := getSortedWordsKey(sr.RawName)
			tr = targetBySorted[sortedKey]
		}
		// 7. Base Product Key (Brand + Modifiers + Letters + Dose) with Pack Compatibility
		if tr == nil {
			srBaseKey, srCount, srHasCount := productmatch.ProductKeyParts(sr.RawName)
			if srBaseKey != "" {
				candidates := targetByBaseKey[srBaseKey]
				if len(candidates) == 1 {
					c := candidates[0]
					if srHasCount && c.hasCount {
						if math.Abs(srCount-c.packCount) < 0.01 {
							tr = c.row
						}
					} else {
						if isPriceCompatible(sr.Price, c.row.Price) {
							tr = c.row
						}
					}
				} else if len(candidates) > 1 {
					if srHasCount {
						for _, c := range candidates {
							if c.hasCount && math.Abs(srCount-c.packCount) < 0.01 {
								tr = c.row
								break
							}
						}
					}
					if tr == nil && sr.Price.IsPositive() {
						var bestCandidate *CompareFileRow
						minDiff := int64(math.MaxInt64)
						for _, c := range candidates {
							if c.row.Price.IsPositive() {
								diff := sr.Price.Minor() - c.row.Price.Minor()
								if diff < 0 {
									diff = -diff
								}
								if diff < minDiff && isPriceCompatible(sr.Price, c.row.Price) {
									minDiff = diff
									bestCandidate = c.row
								}
							}
						}
						if bestCandidate != nil {
							tr = bestCandidate
						}
					}
				}
			}
		}
		// 8. Smart Fuzzy Fallback via productmatch.Index
		if tr == nil && fuzzyIndex != nil {
			mRow := &productmatch.Row{
				Name: sr.RawName,
				SKU:  sr.SKU,
			}
			opts := productmatch.DefaultMatchOptions()
			opts.MinStrong = 0.50
			res := fuzzyIndex.Match(mRow, opts)
			if res.Matched() && res.Level.Settled() {
				if candidate := targetByID[res.ProductID]; candidate != nil {
					if isPriceCompatible(sr.Price, candidate.Price) {
						tr = candidate
					}
				}
			}
		}

		// 9. Short cross-script brand (Mobic / موبيك) with identical doses,
		//    only when that key names one row on each side.
		if tr == nil {
			tr = shortKeys.pair(sr)
		}

		if tr != nil {
			var groupBasis string
			var notComparable bool
			if sr.MatchedProductID != nil && tr.MatchedProductID != nil && *sr.MatchedProductID == *tr.MatchedProductID {
				groupBasis = "canonical_pack"
			} else {
				groupBasis = "source_equivalence"
			}

			// Check pack comparability (e.g. strip vs box without conversion)
			srFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: sr.RawName, SKU: sr.SKU}, nil)
			trFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: tr.RawName, SKU: tr.SKU}, nil)
			if srFacts.Pack != nil && trFacts.Pack != nil {
				outcome, _ := productmatch.PackOutcome(*srFacts.Pack, *trFacts.Pack, nil)
				if outcome == "conflict" || outcome == "needs_relationship" {
					notComparable = true
					groupBasis = "not_comparable"
				}
			}

			price := sr.Price
			if price.IsZero() && tr.Price.IsPositive() {
				price = tr.Price
			}

			sNet := sr.PriceAfterDiscount
			if sNet.IsZero() && price.IsPositive() {
				sNet = CalculatePriceAfterDiscount(price, sr.Discount)
			}
			tNet := tr.PriceAfterDiscount
			if tNet.IsZero() && price.IsPositive() {
				tNet = CalculatePriceAfterDiscount(price, tr.Discount)
			}

			if !notComparable {
				sourceTotal, _ = sourceTotal.Add(sNet)
				targetTotal, _ = targetTotal.Add(tNet)
			}

			var outcome HeadToHeadOutcome
			betterDiff := 0.0
			compDiff := 0.0
			var savingsDiff money.Amount

			if notComparable {
				outcome = OutcomeNotComparable
			} else {
				discDiff := sr.Discount - tr.Discount
				if math.Abs(discDiff) < 0.01 {
					outcome = OutcomeEqual
					equalCount++
				} else if sr.Discount > tr.Discount {
					outcome = OutcomeYourBetter
					betterDiff = math.Round((sr.Discount-tr.Discount)*100.0) / 100.0
					yourBetterCount++
				} else {
					outcome = OutcomeCompetitorBetter
					compDiff = math.Round((tr.Discount-sr.Discount)*100.0) / 100.0
					competitorBetterCount++
				}
				savingsDiff, _ = tNet.Sub(sNet)
			}

			var prodID *int64
			if sr.MatchedProductID != nil && tr.MatchedProductID != nil && *sr.MatchedProductID == *tr.MatchedProductID {
				prodID = sr.MatchedProductID
			}

			row := &HeadToHeadRow{
				ProductID:          prodID,
				ProductName:        sr.RawName,
				SKU:                sr.SKU,
				Price:              price,
				YourDiscount:       sr.Discount,
				YourNetPrice:       sNet,
				CompetitorDiscount: tr.Discount,
				CompetitorNetPrice: tNet,
				Outcome:            outcome,
				BetterDiff:         betterDiff,
				CompetitorDiff:     compDiff,
				SavingsDifference:  savingsDiff,
				GroupBasis:         groupBasis,
				NotComparable:      notComparable,
			}

			// Apply in-memory filters
			if qLower != "" {
				nameMatch := strings.Contains(strings.ToLower(row.ProductName), qLower)
				skuMatch := strings.Contains(strings.ToLower(row.SKU), qLower)
				if !nameMatch && !skuMatch {
					continue
				}
			}

			if filter.MinPrice != nil && row.Price.Minor() < int64(*filter.MinPrice*100) {
				continue
			}
			if filter.MaxPrice != nil && row.Price.Minor() > int64(*filter.MaxPrice*100) {
				continue
			}
			if filter.MinDiscount != nil && (row.YourDiscount < *filter.MinDiscount && row.CompetitorDiscount < *filter.MinDiscount) {
				continue
			}
			if filter.MaxDiscount != nil && (row.YourDiscount > *filter.MaxDiscount && row.CompetitorDiscount > *filter.MaxDiscount) {
				continue
			}
			if filter.Outcome != nil && *filter.Outcome != "" && row.Outcome != *filter.Outcome {
				continue
			}

			allSharedRows = append(allSharedRows, row)
		}
	}

	totalShared := len(allSharedRows)
	comparableCount := yourBetterCount + equalCount + competitorBetterCount
	winRate := 0.0
	if comparableCount > 0 {
		winRate = math.Round((float64(yourBetterCount+equalCount)/float64(comparableCount))*1000.0) / 10.0
	}
	totalSavings, _ := targetTotal.Sub(sourceTotal)

	return &HeadToHeadComparisonResult{
		SourceSupplierName: sourceFile.SupplierName,
		TargetSupplierName: targetFile.SupplierName,
		SourceFileID:       filter.SourceFileID,
		TargetFileID:       filter.TargetFileID,
		Rows:               allSharedRows,
		TotalShared:        totalShared,
		YourBetterCount:    yourBetterCount,
		EqualCount:         equalCount,
		CompetitorBetter:   competitorBetterCount,
		WinRatePercent:     winRate,
		TotalSavings:       totalSavings,
		SourceTotal:        sourceTotal,
		TargetTotal:        targetTotal,
	}, nil
}

type targetRowEntry struct {
	row       *CompareFileRow
	packCount float64
	hasCount  bool
}

// isPriceCompatible checks whether two public prices are within a plausible ratio (0.35x to 2.85x)
// to prevent matching different pack sizes (e.g. 20 tablets vs 200 tablets).
// If either price is zero/unknown, it returns true.
// Uses integer arithmetic on minor units (piastres) in accordance with Rule 1.
func isPriceCompatible(p1, p2 money.Amount) bool {
	if p1.IsZero() || p2.IsZero() {
		return true
	}
	m1 := p1.Minor()
	m2 := p2.Minor()
	if m1 <= 0 || m2 <= 0 {
		return true
	}
	return m1*100 >= m2*35 && m1*100 <= m2*285
}
