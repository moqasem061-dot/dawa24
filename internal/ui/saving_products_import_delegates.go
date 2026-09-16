package ui

import (
	"net/http"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CustomerSavingProductsSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) CustomerSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	lang, _ := h.localeAndDir(r)
	f := excelize.NewFile()
	sheet := "Saving Products Sample"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: boolPtr(true),
	})

	headers := []string{
		i18n.T(lang, "customer.saving.sample_col_name"),
		i18n.T(lang, "customer.saving.sample_col_sku"),
		i18n.T(lang, "customer.saving.sample_col_qty"),
		i18n.T(lang, "customer.saving.sample_col_price"),
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowHeight(sheet, 1, 26)

	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, hName)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	samples := [][]any{
		{i18n.TDefault("w4_ui.500_69"), "PAN-EXT-24", 50, 48.50},
		{i18n.TDefault("w4_ui.s_70_70"), "CONG-TAB-20", 30, 29.00},
		{i18n.TDefault("w4_ui.1_14_71"), "AUG-1G-14", 20, 110.00},
		{i18n.TDefault("w4_ui.200_72"), "ANT-200-24", 40, 32.00},
	}

	for rIdx, row := range samples {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 35)
	_ = f.SetColWidth(sheet, "B", "B", 18)
	_ = f.SetColWidth(sheet, "C", "C", 14)
	_ = f.SetColWidth(sheet, "D", "D", 16)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.xlsx\"")
	_ = f.Write(w)
}

// CustomerSavingProductsSampleCSV streams download of a clean CSV template.
func (h *UIHandler) CustomerSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	lang, _ := h.localeAndDir(r)
	csvContent := "\xEF\xBB\xBF" + i18n.T(lang, "customer.saving.sample_col_name") + "," +
		i18n.T(lang, "customer.saving.sample_col_sku") + "," +
		i18n.T(lang, "customer.saving.sample_col_qty") + "," +
		i18n.T(lang, "customer.saving.sample_col_price") + "\n" +
		i18n.TDefault("w4_ui.500_pan_ext_24_50_48_50_18") +
		i18n.TDefault("w4_ui.cong_tab_20_30_29_00_n_19") +
		i18n.TDefault("w4_ui.1_14_aug_1g_14_20_110_00_20") +
		i18n.TDefault("w4_ui.200_ant_200_24_40_32_00_21")

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.csv\"")
	_, _ = w.Write([]byte(csvContent))
}

// VendorSavingProductsSampleXLSX streams download of a clean Excel template for vendor.
func (h *UIHandler) VendorSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleXLSX(w, r)
}

// VendorSavingProductsSampleCSV streams download of a clean CSV template for vendor.
func (h *UIHandler) VendorSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleCSV(w, r)
}

// Customer Saving Products Import Handlers (public delegates)

func (h *UIHandler) CustomerSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportPage(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportUploadSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSessionPage(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportMapSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemUpdateSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemMatchSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemToggleSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}

// Vendor Saving Products Import Handlers (public delegates)

func (h *UIHandler) VendorSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportPage(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportUploadSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSessionPage(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportMapSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemUpdateSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemMatchSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemToggleSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}
