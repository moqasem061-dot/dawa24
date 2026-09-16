package ui

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Vendor catalog import handlers are implemented in vendor_ingest_handlers.go (Plan V5 Phase 4).

// VendorIngestSampleXLSX streams a styled Excel template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	headers := []string{
		i18n.T("ar", "ingest.col.barcode"),
		i18n.T("ar", "ingest.col.name_ar"),
		i18n.T("ar", "ingest.col.name_en"),
		i18n.T("ar", "ingest.col.scientific_name"),
		i18n.TDefault("w4_ui.s_164_164"),
		i18n.T("ar", "ingest.col.dosage_form"),
		i18n.T("ar", "ingest.col.manufacturer"),
		i18n.TDefault("w4_ui.s_165_165"),
		i18n.TDefault("w4_ui.s_166_166"),
		i18n.T("ar", "ingest.col.batch_no"),
		i18n.T("ar", "ingest.col.expiry_date"),
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowHeight(sheet, 1, 26)

	for i, head := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, head)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	sampleRows := [][]string{
		{"6221142001234", i18n.TDefault("w4_ui.24_167"), "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", i18n.TDefault("w4_ui.s_13_13"), "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", i18n.TDefault("w4_ui.1_14_168"), "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", i18n.TDefault("w4_ui.s_13_13"), "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", i18n.TDefault("w4_ui.50_169"), "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", i18n.TDefault("w4_ui.s_170_170"), "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", i18n.TDefault("w4_ui.20_59"), "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", i18n.TDefault("w4_ui.s_13_13"), "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", i18n.TDefault("w4_ui.24_171"), "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", i18n.TDefault("w4_ui.s_21_21"), "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for rIdx, row := range sampleRows {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	colWidths := []float64{18, 30, 28, 28, 26, 16, 22, 14, 12, 16, 16}
	for i, w := range colWidths {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, colName, colName, w)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.xlsx\"")
	_ = f.Write(w)
}

// VendorIngestSampleCSV streams a UTF-8 BOM CSV template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{
		i18n.T("ar", "ingest.col.barcode"),
		i18n.T("ar", "ingest.col.name_ar"),
		i18n.T("ar", "ingest.col.name_en"),
		i18n.T("ar", "ingest.col.scientific_name"),
		i18n.TDefault("w4_ui.s_164_164"),
		i18n.T("ar", "ingest.col.dosage_form"),
		i18n.T("ar", "ingest.col.manufacturer"),
		i18n.TDefault("w4_ui.s_165_165"),
		i18n.TDefault("w4_ui.s_166_166"),
		i18n.T("ar", "ingest.col.batch_no"),
		i18n.T("ar", "ingest.col.expiry_date"),
	}
	_ = writer.Write(headers)

	sampleRows := [][]string{
		{"6221142001234", i18n.TDefault("w4_ui.24_167"), "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", i18n.TDefault("w4_ui.s_13_13"), "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", i18n.TDefault("w4_ui.1_14_168"), "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", i18n.TDefault("w4_ui.s_13_13"), "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", i18n.TDefault("w4_ui.50_169"), "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", i18n.TDefault("w4_ui.s_170_170"), "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", i18n.TDefault("w4_ui.20_59"), "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", i18n.TDefault("w4_ui.s_13_13"), "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", i18n.TDefault("w4_ui.24_171"), "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", i18n.TDefault("w4_ui.s_21_21"), "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for _, row := range sampleRows {
		_ = writer.Write(row)
	}
}

// VendorIngestExport exports the vendor's real active inventory and pricing as a CSV.
func (h *UIHandler) VendorIngestExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_inventory.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{i18n.TDefault("w4_ui.sku_29"), i18n.TDefault("w4_ui.s_172_172"), i18n.TDefault("w4_ui.w4str_30_30"), i18n.TDefault("w4_ui.w4str_31_31"), i18n.T("ar", "ingest.col.batch_no"), i18n.T("ar", "ingest.col.expiry_date"), i18n.TDefault("w4_ui.s_173_173")}
	_ = writer.Write(headers)

	if h.catSvc != nil {
		products, _ := h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{Limit: 500})
		for _, p := range products {
			if p == nil {
				continue
			}
			variants, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
			for _, v := range variants {
				if v != nil && v.OrganizationID == actor.OrganizationID {
					expStr := ""
					if v.ExpiryDate != nil {
						expStr = v.ExpiryDate.Format("2006-01-02")
					}
					status := i18n.TDefault("w4_ui.s_174_174")
					if v.Status != catalog.StatusActive && v.Status != "" {
						status = i18n.TDefault("w4_ui.s_175_175")
					}
					_ = writer.Write([]string{
						v.SKU,
						p.Name.Get(i18n.AR),
						v.Price.String(),
						fmt.Sprintf("%d", v.StockQty),
						v.BatchNumber,
						expStr,
						status,
					})
				}
			}
		}
	}
}
