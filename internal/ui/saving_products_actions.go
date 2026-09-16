package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// handleSavingProductsBulkDeleteSubmit deletes multiple selected saving product records for customer or vendor.
func (h *UIHandler) handleSavingProductsBulkDeleteSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", "بيانات الطلب غير صالحة")
		return
	}

	rawIDs := r.Form["selected_ids"]
	if len(rawIDs) == 0 {
		rawIDs = r.Form["ids"]
	}
	if len(rawIDs) == 0 {
		if single := strings.TrimSpace(r.FormValue("selected_ids")); single != "" {
			rawIDs = strings.Split(single, ",")
		} else if single := strings.TrimSpace(r.FormValue("ids")); single != "" {
			rawIDs = strings.Split(single, ",")
		}
	}

	var ids []int64
	for _, raw := range rawIDs {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
	}

	if len(ids) == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", "لم يتم تحديد أي أصناف لحذفها")
		return
	}

	var deletedCount int
	if h.catSvc != nil {
		for _, id := range ids {
			if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err == nil {
				deletedCount++
			}
		}
	}

	if deletedCount == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", "تعذر حذف الأصناف المحددة، أو تم حذفها مسبقاً")
		return
	}

	msg := fmt.Sprintf("تم حذف %d صنف من قائمة التوفير بنجاح", deletedCount)
	h.redirectWithNotice(w, r, targetURL, "success", msg)
}

// handleSavingProductDeleteAllSubmit deletes all saving products for the organization.
func (h *UIHandler) handleSavingProductDeleteAllSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteAllSavingProducts(ctx, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("delete all %s saving products error", audience), "error", err)
			h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.delete_all_error"))
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.delete_all_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.delete_all_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductsExport streams an Excel spreadsheet of all saving products for customer or vendor.
func (h *UIHandler) handleSavingProductsExport(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/saving-products", audience), http.StatusSeeOther)
		return
	}

	var items []*catalog.SavingProductEnriched
	if h.catSvc != nil {
		it, _, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, "", "all", 2000, 0)
		if err == nil {
			items = it
		}
	}

	f := excelize.NewFile()
	sheet := "Saving Products"
	f.SetSheetName("Sheet1", sheet)

	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	var headers []string
	var filename string
	if audience == "vendor" {
		filename = fmt.Sprintf("saving_products_org_%d.xlsx", actor.OrganizationID)
		headers = []string{
			i18n.T(lang, "customer.saving.export_col_id"),
			i18n.T(lang, "vendor.saving.export_col_name"),
			i18n.T(lang, "customer.saving.export_col_sku"),
			i18n.T(lang, "customer.saving.export_col_qty"),
			i18n.T(lang, "vendor.saving.export_col_price"),
			i18n.T(lang, "customer.saving.export_col_total"),
			i18n.T(lang, "customer.saving.export_col_product_id"),
			i18n.T(lang, "customer.saving.export_col_linked_name"),
			i18n.T(lang, "vendor.saving.export_col_orgs"),
		}
	} else {
		filename = fmt.Sprintf("pharmacy_saving_products_%d.xlsx", actor.OrganizationID)
		headers = []string{
			i18n.T(lang, "customer.saving.export_col_id"),
			i18n.T(lang, "customer.saving.export_col_name"),
			i18n.T(lang, "customer.saving.export_col_sku"),
			i18n.T(lang, "customer.saving.export_col_qty"),
			i18n.T(lang, "customer.saving.export_col_price"),
			i18n.T(lang, "customer.saving.export_col_total"),
			i18n.T(lang, "customer.saving.export_col_product_id"),
			i18n.T(lang, "customer.saving.export_col_linked_name"),
			i18n.T(lang, "customer.saving.export_col_suppliers"),
		}
	}

	for colIdx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, header)
	}

	for rowIdx, it := range items {
		rNum := rowIdx + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), it.ID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), it.NameProduct)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), it.SKU)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), it.Quantity)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), it.Price.String())
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), it.TotalValue.String())

		if it.ProductID != nil && *it.ProductID > 0 {
			_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), *it.ProductID)
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), it.LinkedProductName.Get(i18n.ParseLang(lang)))
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), it.ProvidingOrgsCount)
		} else {
			_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), "")
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), i18n.T(lang, "customer.saving.not_linked"))
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), 0)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}

// SavingProductProvidersJSON returns JSON array of suppliers selling a master product.
func (h *UIHandler) SavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	productID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || productID <= 0 {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	if h.catSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	providers, err := h.catSvc.GetProductProviders(ctx, productID)
	if err != nil {
		h.log.ErrorContext(ctx, "get product providers error", "error", err, "product_id", productID)
		providers = []*catalog.ProductProviderInfo{}
	}

	type jsonProvider struct {
		OrgID              int64  `json:"org_id"`
		OrgName            string `json:"org_name"`
		VariantName        string `json:"variant_name"`
		SKU                string `json:"sku"`
		Unit               string `json:"unit"`
		Price              string `json:"price"`
		Discount           string `json:"discount"`
		PriceAfterDiscount string `json:"price_after_discount"`
		StockQuantity      int    `json:"stock_quantity"`
		BranchName         string `json:"branch_name"`
	}

	var res []jsonProvider
	for _, p := range providers {
		res = append(res, jsonProvider{
			OrgID:              p.OrgID,
			OrgName:            p.OrgName.Get(i18n.ParseLang(langOf(r))),
			VariantName:        p.VariantName.Get(i18n.ParseLang(langOf(r))),
			SKU:                p.SKU,
			Unit:               p.Unit,
			Price:              p.Price.String(),
			Discount:           p.Discount.String(),
			PriceAfterDiscount: p.PriceAfterDiscount.String(),
			StockQuantity:      p.StockQuantity,
			BranchName:         p.BranchName.Get(i18n.ParseLang(langOf(r))),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

// SavingProductSearchJSON returns JSON autocomplete list of catalog products for linking.
// Includes Name, Label, NameAR, and NameEN so decision memory relink modals always display clean names.
func (h *UIHandler) SavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type searchResult struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Label  string `json:"label"`
		NameAR string `json:"name_ar,omitempty"`
		NameEN string `json:"name_en,omitempty"`
		SKU    string `json:"sku,omitempty"`
	}

	var results []searchResult
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Query:     query,
			FirstWord: catalog.FirstWordOf(query),
			Limit:     20,
		})
		if err == nil {
			for _, p := range products {
				nameAR := p.Name.Get(i18n.AR)
				nameEN := p.Name.Get(i18n.EN)
				name := p.Name.Get(i18n.ParseLang(langOf(r)))
				if name == "" {
					name = nameAR
				}
				if name == "" {
					name = nameEN
				}
				results = append(results, searchResult{
					ID:     fmt.Sprintf("%d", p.ID),
					Name:   name,
					Label:  name,
					NameAR: nameAR,
					NameEN: nameEN,
					SKU:    p.SKU,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}
