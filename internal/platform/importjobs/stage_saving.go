package importjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SavingPayload is the JSONB payload stored on import_runs for saving_products.
type SavingPayload struct {
	Headers      []string   `json:"headers"`
	SampleRows   [][]string `json:"sample_rows,omitempty"`
	RawDataRows  [][]string `json:"raw_data_rows"`
	NameCol      int        `json:"name_col"`
	SKUCol       int        `json:"sku_col"`
	QtyCol       int        `json:"qty_col"`
	PriceCol     int        `json:"price_col"`
	ProductIDCol int        `json:"product_id_col"`
	MatchChoice  any        `json:"match_choice,omitempty"`
	UseAI        bool       `json:"use_ai,omitempty"`
	Lang         string     `json:"lang,omitempty"`
}

// SavingRowData is what goes into import_run_rows.data for each staged row.
type SavingRowData struct {
	Index             int     `json:"index"`
	NameProduct       string  `json:"name_product"`
	SKU               string  `json:"sku"`
	Quantity          float64 `json:"quantity"`
	PriceMinor        int64   `json:"price_minor"`
	TotalValueMinor   int64   `json:"total_value_minor"`
	ProductID         *int64  `json:"product_id,omitempty"`
	MasterProductName string  `json:"master_product_name,omitempty"`
	MasterProductSKU  string  `json:"master_product_sku,omitempty"`
	MatchType         string  `json:"match_type"`
	Confidence        float64 `json:"confidence"`
}

// SavingResult is the summary stored in import_runs.result.
type SavingResult struct {
	MatchedRows   int   `json:"matched_rows"`
	UnlinkedRows  int   `json:"unlinked_rows"`
	TotalQuantity int64 `json:"total_quantity_x100"` // ×100 for precision
	TotalMinor    int64 `json:"total_minor"`
	InsertedCount int   `json:"inserted_count,omitempty"`
	UpdatedCount  int   `json:"updated_count,omitempty"`
}

// stageSavingProducts processes a saving_products import run.
func (w *StageWorker) stageSavingProducts(ctx context.Context, run *importrun.Run) error {
	// Decode the payload.
	var payload SavingPayload
	if err := json.Unmarshal(run.Payload, &payload); err != nil {
		return fmt.Errorf("decode saving payload: %w", err)
	}

	dataRows := payload.RawDataRows
	if len(dataRows) == 0 {
		return fmt.Errorf("no data rows in payload")
	}

	nCol := payload.NameCol
	sCol := payload.SKUCol
	qCol := payload.QtyCol
	pCol := payload.PriceCol
	pidCol := payload.ProductIDCol

	// Load catalogue for matching.
	var matchProducts []matchProduct
	if w.catSvc != nil {
		catalogSources, err := w.catSvc.ListMatchProducts(ctx)
		if err == nil && len(catalogSources) > 0 {
			for _, p := range catalogSources {
				name := p.NameAR
				if name == "" {
					name = p.NameEN
				}
				matchProducts = append(matchProducts, matchProduct{
					ID:   p.ID,
					Name: name,
					SKU:  p.SKU,
				})
			}
		}
	}

	_ = w.repo.UpdateProgress(ctx, run.ID, "جارٍ مطابقة الأصناف...", 30, 0)

	total := len(dataRows)
	var stagedRows []importrun.Row
	matchedCount := 0
	unlinkedCount := 0
	var totalQty float64
	var totalValMinor int64

	for i, row := range dataRows {
		if len(row) == 0 || isAllEmpty(row) || isSummaryRow(row) {
			continue
		}

		var name string
		if nCol >= 0 && nCol < len(row) {
			name = strings.TrimSpace(row[nCol])
		}
		var sku string
		if sCol >= 0 && sCol < len(row) {
			sku = strings.TrimSpace(row[sCol])
		}

		// Content swap heuristic.
		if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveText(sku) {
			name, sku = sku, name
		}
		if name == "" && sku != "" {
			name = sku
		}
		if name == "" {
			continue
		}

		var qty float64
		if qCol >= 0 && qCol < len(row) {
			qty = parseFlexQty(row[qCol])
		}

		var priceMinor int64
		if pCol >= 0 && pCol < len(row) {
			if p, err := money.Parse(row[pCol]); err == nil {
				priceMinor = p.Minor()
			}
		}

		var productID *int64
		if pidCol >= 0 && pidCol < len(row) {
			if pid, err := strconv.ParseInt(strings.TrimSpace(row[pidCol]), 10, 64); err == nil && pid > 0 {
				productID = &pid
			}
		}

		// Simple name matching against catalogue.
		matchType := "unlinked"
		confidence := 0.0
		masterName := ""
		masterSKU := ""
		if len(matchProducts) > 0 && productID == nil {
			bestID, bestScore, bName, bSKU := findBestMatch(name, sku, matchProducts)
			if bestScore >= 0.65 {
				productID = &bestID
				matchType = "name_match"
				confidence = bestScore
				masterName = bName
				masterSKU = bSKU
			}
		}

		if productID != nil {
			matchedCount++
			if masterName == "" {
				for _, p := range matchProducts {
					if p.ID == *productID {
						masterName = p.Name
						masterSKU = p.SKU
						break
					}
				}
			}
		} else {
			unlinkedCount++
		}

		rowTotalMinor := int64(qty * float64(priceMinor))
		totalQty += qty
		totalValMinor += rowTotalMinor

		rowData := SavingRowData{
			Index:             len(stagedRows) + 1,
			NameProduct:       name,
			SKU:               sku,
			Quantity:          qty,
			PriceMinor:        priceMinor,
			TotalValueMinor:   rowTotalMinor,
			ProductID:         productID,
			MasterProductName: masterName,
			MasterProductSKU:  masterSKU,
			MatchType:         matchType,
			Confidence:        confidence,
		}

		data, err := json.Marshal(rowData)
		if err != nil {
			return fmt.Errorf("marshal row data: %w", err)
		}

		stagedRows = append(stagedRows, importrun.Row{
			RunID:            run.ID,
			RowNumber:        len(stagedRows) + 1,
			Data:             data,
			Included:         true,
			MatchedProductID: productID,
		})

		// Update progress every 100 rows.
		if i%100 == 0 || i == total-1 {
			pct := 30 + int(float64(i+1)/float64(total)*65)
			if pct > 98 {
				pct = 98
			}
			phase := fmt.Sprintf("تمت معالجة %d من %d صنف", i+1, total)
			_ = w.repo.UpdateProgress(ctx, run.ID, phase, pct, i+1)
		}
	}

	// Persist staged rows.
	if err := w.repo.InsertRows(ctx, run.ID, stagedRows); err != nil {
		return fmt.Errorf("insert staged rows: %w", err)
	}

	// Store result summary.
	result := SavingResult{
		MatchedRows:   matchedCount,
		UnlinkedRows:  unlinkedCount,
		TotalQuantity: int64(totalQty * 100),
		TotalMinor:    totalValMinor,
	}
	resultJSON, _ := json.Marshal(result)
	if err := w.repo.SetResult(ctx, run.ID, resultJSON); err != nil {
		return fmt.Errorf("set result: %w", err)
	}

	// Transition to ready.
	_ = w.repo.UpdateProgress(ctx, run.ID, "اكتملت المعالجة", 100, total)
	return w.repo.TransitionState(ctx, run.ID, importrun.StateReady)
}
