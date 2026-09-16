package compare

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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
	targetByName := make(map[string]*CompareFileRow)
	targetByCore := make(map[string]*CompareFileRow)
	targetBySorted := make(map[string]*CompareFileRow)
	targetByProductID := make(map[int64]*CompareFileRow)

	for _, tr := range targetRows {
		if tr.MatchedProductID != nil && *tr.MatchedProductID > 0 {
			targetByProductID[*tr.MatchedProductID] = tr
		}
		cleanSKU := strings.ToLower(strings.TrimSpace(tr.SKU))
		if cleanSKU != "" {
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
	}

	var allSharedRows []*HeadToHeadRow
	yourBetterCount := 0
	equalCount := 0
	competitorBetterCount := 0
	var sourceTotal money.Amount
	var targetTotal money.Amount

	qLower := strings.ToLower(strings.TrimSpace(filter.Query))

	for _, sr := range sourceRows {
		var tr *CompareFileRow
		if sr.MatchedProductID != nil && *sr.MatchedProductID > 0 {
			tr = targetByProductID[*sr.MatchedProductID]
		}
		if tr == nil {
			cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))
			if cleanSKU != "" {
				tr = targetBySKU[cleanSKU]
			}
		}
		if tr == nil {
			norm := normalizeProductText(sr.RawName)
			tr = targetByName[norm]
		}
		if tr == nil {
			coreKey := getCoreDrugMatchKey(sr.RawName)
			tr = targetByCore[coreKey]
		}
		if tr == nil {
			sortedKey := getSortedWordsKey(sr.RawName)
			tr = targetBySorted[sortedKey]
		}

		if tr != nil {
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

			sourceTotal, _ = sourceTotal.Add(sNet)
			targetTotal, _ = targetTotal.Add(tNet)

			var outcome HeadToHeadOutcome
			betterDiff := 0.0
			compDiff := 0.0

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

			savingsDiff, _ := tNet.Sub(sNet)

			row := &HeadToHeadRow{
				ProductID:          sr.MatchedProductID,
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

	totalShared := yourBetterCount + equalCount + competitorBetterCount
	winRate := 0.0
	if totalShared > 0 {
		winRate = math.Round((float64(yourBetterCount+equalCount)/float64(totalShared))*1000.0) / 10.0
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
