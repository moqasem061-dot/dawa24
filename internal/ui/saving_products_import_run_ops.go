package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importjobs"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// startSavingImportRun coordinates asynchronous processing of saving products imports.
// It creates a durable platform.import_runs entry, mirrors it to the in-memory
// store for transitional UI compatibility, and starts background processing.
func (h *UIHandler) startSavingImportRun(
	ctx context.Context,
	actor authctx.Actor,
	filename string,
	rawRows [][]string,
	headers []string,
	sampleRows [][]string,
	nCol, sCol, qCol, pCol, pidCol int,
	choice MatchChoice,
	useAI bool,
	lang string,
	audience string,
) (string, int, error) {
	totalRows := len(rawRows) - 1
	if totalRows <= 0 {
		return "", 0, fmt.Errorf("%s", i18n.T(lang, "customer.saving.import.file_empty_no_rows"))
	}

	dataRows := rawRows[1:]

	payload := importjobs.SavingPayload{
		Headers:      headers,
		SampleRows:   sampleRows,
		RawDataRows:  dataRows,
		NameCol:      nCol,
		SKUCol:       sCol,
		QtyCol:       qCol,
		PriceCol:     pCol,
		ProductIDCol: pidCol,
		MatchChoice:  choice,
		UseAI:        useAI,
		Lang:         lang,
	}
	payloadBytes, _ := json.Marshal(payload)

	var runID int64
	var publicID string

	if h.importRunRepo != nil {
		run := &importrun.Run{
			OrganizationID: actor.OrganizationID,
			UserID:         actor.UserID,
			Kind:           importrun.KindSavingProducts,
			Audience:       audience,
			Filename:       filename,
			State:          importrun.StateProcessing,
			Phase:          i18n.T(lang, "customer.saving.import.progress_loading_catalog"),
			Percent:        5,
			TotalRows:      totalRows,
			ProcessedRows:  0,
			Payload:        payloadBytes,
		}
		if err := h.importRunRepo.CreateRun(ctx, run); err != nil {
			h.log.WarnContext(ctx, "failed to create durable import run for saving products", "error", err)
		} else {
			runID = run.ID
			publicID = run.PublicID
		}
	}

	if publicID != "" {
		globalSavingImportSessionStore.NewSessionWithID(publicID, actor.OrganizationID, actor.UserID, filename, totalRows)
	} else {
		memSession := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, filename, totalRows)
		publicID = memSession.ID
	}

	go func(runID int64, sessID string, orgID, userID int64, rows [][]string, nC, sC, qC, pC, pidC int, ch MatchChoice, aiOn bool, l string) {
		bgCtx := database.WithTenant(context.Background(), orgID)
		bgCtx = authctx.WithActor(bgCtx, authctx.Actor{
			UserID:         userID,
			OrganizationID: orgID,
			OrgID:          orgID,
		})
		total := len(rows)

		// A panic in here used to take the whole web process down with it: this
		// goroutine is detached from the request, so nothing above it could
		// recover. It also had no failure path at all — an error mid-run left
		// the session on `processing` and its progress bar frozen, which is
		// indistinguishable to the buyer from the run still working.
		//
		// Both are handled here: the run is marked failed, on the session and
		// on the durable row, and the watching bar is told so it stops rather
		// than drifting forever.
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			h.log.Error("saving products import panicked",
				"session_id", sessID, "run_id", runID, "org_id", orgID, "panic", rec)
			h.failSavingImportRun(bgCtx, runID, sessID, total,
				i18n.T(l, "customer.saving.import.processing_failed"))
		}()

		publishProgress := func(pct int, msg string, cur int) {
			globalSavingImportSessionStore.UpdateProgress(sessID, pct, msg, cur)
			if h.importRunRepo != nil && runID > 0 {
				_ = h.importRunRepo.UpdateProgress(bgCtx, runID, msg, pct, cur)
			}
			if h.progressHub != nil {
				h.progressHub.Publish(progress.Snapshot{
					ID:      sessID,
					Percent: pct,
					Message: msg,
					Current: cur,
					Total:   total,
					State:   "processing",
					Done:    false,
					At:      time.Now(),
				})
			}
		}

		publishRowProgress := func(i int) {
			if total <= 0 || i < 0 {
				return
			}
			var pct int
			if aiOn {
				pct = 20 + int(float64(i+1)/float64(total)*45)
				if pct > 65 {
					pct = 65
				}
			} else {
				pct = 20 + int(float64(i+1)/float64(total)*75)
				if pct > 95 {
					pct = 95
				}
			}
			publishProgress(pct, fmt.Sprintf(i18n.T(l, "customer.saving.import.progress_processed"), i+1, total), i+1)
		}

		phaseLoading := i18n.T(l, "customer.saving.import.progress_loading_catalog")
		publishProgress(15, phaseLoading, 0)

		var matchEngine *SavingProductMatchEngine
		if h.catSvc != nil {
			catalogSources, err := h.catSvc.ListMatchProducts(bgCtx)
			switch {
			case err != nil:
				// Every row would come back unlinked, and the screen would
				// blame the pharmacy's file for it. Fail the run instead: a
				// stated failure is recoverable, a silent one is a support
				// ticket that starts "nothing matched".
				h.log.ErrorContext(bgCtx, "saving products import could not load the catalogue",
					"session_id", sessID, "run_id", runID, "error", err)
				h.failSavingImportRun(bgCtx, runID, sessID, total,
					i18n.T(l, "customer.saving.import.catalog_unavailable"))
				return
			case len(catalogSources) == 0:
				h.log.ErrorContext(bgCtx, "saving products import found an empty catalogue",
					"session_id", sessID, "run_id", runID)
			default:
				matchEngine = NewSavingProductMatchEngine(catalogSources)
			}
		}

		stagedItems := make([]*StagedSavingItem, 0, total)
		var dbRows []importrun.Row
		matchedCount := 0
		unlinkedCount := 0
		var totalQty float64
		var totalValMinor int64

		phaseMatching := i18n.T(l, "customer.saving.import.progress_matching")
		publishProgress(30, phaseMatching, 0)

		for i, row := range rows {
			if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
				continue
			}

			var name string
			if nC >= 0 && nC < len(row) {
				name = strings.TrimSpace(row[nC])
			}
			var sku string
			if sC >= 0 && sC < len(row) {
				sku = strings.TrimSpace(row[sC])
			}

			if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveArabicText(sku) {
				name, sku = sku, name
			}
			if name == "" && sku != "" {
				name = sku
			}
			if name == "" {
				continue
			}

			var qty float64 = 1.0
			if qC >= 0 && qC < len(row) {
				if parsedQ, ok := ParseFlexibleQuantity(row[qC]); ok && parsedQ > 0 {
					qty = parsedQ
				}
			}

			var price money.Amount
			if pC >= 0 && pC < len(row) {
				price, _ = ParseFlexibleMoney(row[pC])
			}

			var productID *int64
			if pidC >= 0 && pidC < len(row) {
				if pid, err := strconv.ParseInt(strings.TrimSpace(row[pidC]), 10, 64); err == nil && pid > 0 {
					productID = &pid
				}
			}

			matchType := "unlinked"
			confidence := 0.0
			if matchEngine != nil {
				res := matchEngine.MatchUnified(ch, productID, sku, name)
				if res.ProductID != nil {
					productID = res.ProductID
					matchType = res.MatchType
					confidence = res.Confidence
				}
			}

			masterName := ""
			masterSKU := ""
			if productID != nil {
				matchedCount++
				if matchEngine != nil {
					masterName, masterSKU = matchEngine.Describe(*productID)
				}
			} else {
				unlinkedCount++
			}

			rowTotalMinor := int64(qty * float64(price.Minor()))
			totalQty += qty
			totalValMinor += rowTotalMinor

			item := &StagedSavingItem{
				Index:             len(stagedItems) + 1,
				NameProduct:       name,
				SKU:               sku,
				Quantity:          qty,
				Price:             price,
				TotalValue:        money.FromMinor(rowTotalMinor),
				ProductID:         productID,
				MasterProductName: masterName,
				MasterProductSKU:  masterSKU,
				MatchType:         matchType,
				Confidence:        confidence,
				Included:          true,
			}
			stagedItems = append(stagedItems, item)

			if i%100 == 0 || i == total-1 {
				publishRowProgress(i)
			}
		}
		// The loop above reports from inside the body, so a file whose last
		// rows are blank or a totals line never reached its final report and
		// the bar stopped short of the matching stage's end.
		publishRowProgress(total - 1)

		if aiOn {
			publishProgress(68, i18n.T(l, "customer.saving.import.progress_matching"), total)
		}

		aiProgress := func(doneBatches, totalBatches, improved int) {
			if totalBatches <= 0 {
				return
			}
			pct := 68 + int(float64(doneBatches)/float64(totalBatches)*28)
			if pct > 96 {
				pct = 96
			}
			msg := fmt.Sprintf(i18n.T(l, "customer.saving.import.progress_ai_batch"), doneBatches, totalBatches)
			publishProgress(pct, msg, doneBatches)
		}

		h.enhanceSavingWithProgress(bgCtx, aiOn, matchEngine, stagedItems, aiProgress)

		matchedCount = 0
		unlinkedCount = 0
		if h.importRunRepo != nil && runID > 0 {
			dbRows = make([]importrun.Row, 0, len(stagedItems))
		}
		for _, item := range stagedItems {
			if item.ProductID != nil {
				matchedCount++
			} else {
				unlinkedCount++
			}
			if h.importRunRepo != nil && runID > 0 {
				dataBytes, _ := json.Marshal(item)
				dbRows = append(dbRows, importrun.Row{
					RunID:            runID,
					RowNumber:        item.Index,
					Data:             dataBytes,
					Included:         item.Included,
					MatchedProductID: item.ProductID,
				})
			}
		}

		publishProgress(98, i18n.T(l, "customer.saving.import.progress_matching"), total)

		globalSavingImportSessionStore.CompleteProcessing(
			sessID,
			stagedItems,
			matchedCount,
			unlinkedCount,
			totalQty,
			money.FromMinor(totalValMinor),
		)

		if h.importRunRepo != nil && runID > 0 {
			_ = h.importRunRepo.InsertRows(bgCtx, runID, dbRows)
			resCounters := map[string]any{
				"matched_rows":   matchedCount,
				"unlinked_rows":  unlinkedCount,
				"total_quantity": totalQty,
				"total_minor":    totalValMinor,
			}
			resBytes, _ := json.Marshal(resCounters)
			_ = h.importRunRepo.SetResult(bgCtx, runID, resBytes)
			_ = h.importRunRepo.UpdateProgress(bgCtx, runID, "اكتملت المعالجة", 100, total)
			_ = h.importRunRepo.TransitionState(bgCtx, runID, importrun.StateReady)
		}

		if h.progressHub != nil {
			h.progressHub.Publish(progress.Snapshot{
				ID:      sessID,
				Percent: 100,
				Message: i18n.T(l, "ops.saving.processing_complete"),
				Current: total,
				Total:   total,
				State:   "ready",
				Done:    true,
				At:      time.Now(),
			})
		}
	}(runID, publicID, actor.OrganizationID, actor.UserID, dataRows, nCol, sCol, qCol, pCol, pidCol, choice, useAI, lang)

	return publicID, totalRows, nil
}

// failSavingImportRun records a run that could not finish, everywhere the buyer
// might be looking.
//
// Three places, because three of them answer the progress bar: the in-memory
// session the wizard reads, the durable row that outlives a restart, and the
// live stream a watching page is subscribed to. A run marked failed in only one
// of them leaves the other two claiming it is still working.
func (h *UIHandler) failSavingImportRun(ctx context.Context, runID int64, sessID string, total int, msg string) {
	globalSavingImportSessionStore.FailSession(sessID, msg)

	if h.importRunRepo != nil && runID > 0 {
		if err := h.importRunRepo.FailRun(ctx, runID, msg); err != nil {
			h.log.WarnContext(ctx, "could not mark saving import run failed", "run_id", runID, "error", err)
		}
	}

	if h.progressHub != nil {
		h.progressHub.Publish(progress.Snapshot{
			ID:      sessID,
			Message: msg,
			Total:   total,
			State:   "failed",
			Done:    true,
			Error:   msg,
			At:      time.Now(),
		})
	}
}
