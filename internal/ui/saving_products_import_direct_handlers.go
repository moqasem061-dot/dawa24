package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

func (h *UIHandler) handleSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	targetURL := fmt.Sprintf("/%s/saving-products", audience)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.read_error"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(header.Filename) {
		h.redirectWithNotice(w, r, targetURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, header.Filename); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", header.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, targetURL, "error", msg)
		return
	}

	headers := rawRows[0]
	colNameOverride := strings.TrimSpace(r.FormValue("col_name"))
	colSKUOverride := strings.TrimSpace(r.FormValue("col_sku"))
	colQtyOverride := strings.TrimSpace(r.FormValue("col_qty"))
	colPriceOverride := strings.TrimSpace(r.FormValue("col_price"))
	colProductIDOverride := strings.TrimSpace(r.FormValue("col_product_id"))

	sampleRows := rawRows[1:]
	if len(sampleRows) > 10 {
		sampleRows = sampleRows[:10]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		colNameOverride,
		colSKUOverride,
		colQtyOverride,
		colPriceOverride,
		colProductIDOverride,
	)

	matchChoice := ParseMatchChoice(r)

	var matchEngine *SavingProductMatchEngine
	if h.catSvc != nil {
		if catalogSources, err := h.catSvc.ListMatchProducts(ctx); err == nil && len(catalogSources) > 0 {
			matchEngine = NewSavingProductMatchEngine(catalogSources)
		}
	}

	var parsedItems []*catalog.SavingProduct
	var matchedCount int
	var unlinkedCount int

	for i := 1; i < len(rawRows); i++ {
		row := rawRows[i]
		if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
			continue
		}

		var name string
		if nameCol >= 0 && nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}

		var sku string
		if skuCol >= 0 && skuCol < len(row) {
			sku = strings.TrimSpace(row[skuCol])
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

		var qty float64
		if qtyCol >= 0 && qtyCol < len(row) {
			qty, _ = ParseFlexibleQuantity(row[qtyCol])
		}

		var price money.Amount
		if priceCol >= 0 && priceCol < len(row) {
			price, _ = ParseFlexibleMoney(row[priceCol])
		}

		var productID *int64
		if productIDCol >= 0 && productIDCol < len(row) {
			if pid, err := strconv.ParseInt(strings.TrimSpace(row[productIDCol]), 10, 64); err == nil && pid > 0 {
				productID = &pid
			}
		}

		if matchEngine != nil {
			matchRes := matchEngine.MatchUnified(matchChoice, productID, sku, name)
			if matchRes.ProductID != nil {
				productID = matchRes.ProductID
			}
		}

		if productID != nil {
			matchedCount++
		} else {
			unlinkedCount++
		}

		parsedItems = append(parsedItems, &catalog.SavingProduct{
			OrganizationID: actor.OrganizationID,
			UserID:         &actor.UserID,
			ProductID:      productID,
			NameProduct:    name,
			SKU:            sku,
			Quantity:       qty,
			Price:          price,
		})
	}

	if len(parsedItems) == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.no_valid_products"))
		return
	}

	added, updated, err := h.catSvc.BatchUpsertSavingProducts(ctx, actor.OrganizationID, &actor.UserID, parsedItems)
	if err != nil {
		h.log.ErrorContext(ctx, fmt.Sprintf("%s failed to batch upsert saving products", audience), "error", err, "org_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, targetURL, "error", fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.save_error"), h.safeMessage(err, langOf(r))))
		return
	}

	successMsg := fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.success_summary"), len(parsedItems), added, updated, matchedCount, unlinkedCount)
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}
