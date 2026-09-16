package ui

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminProductsPage renders the master products catalog with full-database search and pagination.
func (h *UIHandler) AdminProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("search"))
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "all" {
		status = ""
	}
	dosage := strings.TrimSpace(r.URL.Query().Get("dosage"))
	if dosage == "all" {
		dosage = ""
	}
	manufacturer := strings.TrimSpace(r.URL.Query().Get("manufacturer"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "newest"
	}

	var brandIDPtr *int64
	if bStr := strings.TrimSpace(r.URL.Query().Get("brand_id")); bStr != "" && bStr != "0" {
		if bid, err := strconv.ParseInt(bStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var catIDPtr *int64
	if cStr := strings.TrimSpace(r.URL.Query().Get("category_id")); cStr != "" && cStr != "0" {
		if cid, err := strconv.ParseInt(cStr, 10, 64); err == nil && cid > 0 {
			catIDPtr = &cid
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var products []*catalog.Product
	var totalProducts int
	var brands []*catalog.Brand
	var categories []*catalog.Category
	var dosageForms []string

	if h.catSvc != nil {
		sysCtx := database.AsSystem(ctx)
		prods, total, err := h.catSvc.SearchWithTotal(sysCtx, catalog.SearchParams{
			Query:        q,
			CategoryID:   catIDPtr,
			BrandID:      brandIDPtr,
			Manufacturer: manufacturer,
			Status:       status,
			DosageForm:   dosage,
			Limit:        limit,
			Offset:       offset,
			Sort:         sort,
		})
		if err == nil {
			products = prods
			totalProducts = total
		} else {
			h.log.ErrorContext(ctx, "admin products search failed", "error", err)
		}
		brands, _ = h.catSvc.ListBrands(sysCtx)
		categories, _ = h.catSvc.ListCategories(sysCtx)
		dosageForms, _ = h.catSvc.ListDosageForms(sysCtx)
	}

	var brandFilterVal int64
	if brandIDPtr != nil {
		brandFilterVal = *brandIDPtr
	}
	var catFilterVal int64
	if catIDPtr != nil {
		catFilterVal = *catIDPtr
	}

	h.renderPage(ctx, w, "render admin products", pages.AdminProducts(
		lang, dir, products, brands, categories, dosageForms,
		totalProducts, page, limit,
		q, status, dosage, manufacturer, sort,
		brandFilterVal, catFilterVal,
	))
}

// AdminProductStatusSubmit sets a product's moderation status.
func (h *UIHandler) AdminProductStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		_, _ = h.catSvc.SetProductsStatus(database.AsSystem(ctx), []int64{id}, catalog.ProductStatus(r.PostFormValue("status")))
	}
	http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
}

// AdminProductCreateSubmit creates a new master product from Super Admin dashboard.
func (h *UIHandler) AdminProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.service_unavailable"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}

	var brandIDPtr *int64
	if brandIDStr := strings.TrimSpace(r.FormValue("brand_id")); brandIDStr != "" {
		if bid, err := strconv.ParseInt(brandIDStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var categoryIDPtr *int64
	if catIDStr := strings.TrimSpace(r.FormValue("category_id")); catIDStr != "" {
		if cid, err := strconv.ParseInt(catIDStr, 10, 64); err == nil && cid > 0 {
			categoryIDPtr = &cid
		}
	}

	manufacturer := strings.TrimSpace(r.FormValue("manufacturer"))
	if manufacturer == "" && brandIDPtr != nil {
		if b, err := h.catSvc.GetBrand(database.AsSystem(ctx), *brandIDPtr); err == nil && b != nil {
			manufacturer = b.Name.Get("ar")
			if manufacturer == "" {
				manufacturer = b.Name.Get("en")
			}
		}
	}

	prod := &catalog.Product{
		Name:                   i18n.New(nameAr, nameEn),
		Description:            i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		ScientificName:         r.FormValue("generic_name"),
		Active:                 r.FormValue("active_ingredient"),
		DosageForm:             r.FormValue("dosage_form"),
		BrandID:                brandIDPtr,
		CategoryID:             categoryIDPtr,
		ManufacturingCompanies: manufacturer,
		SKU:                    r.FormValue("eda_reg_number"),
		Barcode:                r.FormValue("eda_reg_number"),
		Image:                  imgURL,
		Status:                 catalog.StatusActive,
		InstitutionalWorkIDs:   []int64{},
	}

	if _, err := h.catSvc.CreateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin create product failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", i18n.T(lang, "admin.products.created_success"))
}

// AdminProductEditSubmit updates an existing master medicine in the catalog.
func (h *UIHandler) AdminProductEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.invalid_id"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), id)
	if err != nil || prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.not_found"))
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}
	if imgURL == "" {
		imgURL = prod.Image
	}

	priceVal, _ := money.Parse(r.FormValue("price"))

	var brandIDPtr *int64
	if brandIDStr := strings.TrimSpace(r.FormValue("brand_id")); brandIDStr != "" {
		if bid, err := strconv.ParseInt(brandIDStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var categoryIDPtr *int64
	if catIDStr := strings.TrimSpace(r.FormValue("category_id")); catIDStr != "" {
		if cid, err := strconv.ParseInt(catIDStr, 10, 64); err == nil && cid > 0 {
			categoryIDPtr = &cid
		}
	}

	manufacturer := strings.TrimSpace(r.FormValue("manufacturer"))
	if manufacturer == "" && brandIDPtr != nil {
		if b, err := h.catSvc.GetBrand(database.AsSystem(ctx), *brandIDPtr); err == nil && b != nil {
			manufacturer = b.Name.Get("ar")
			if manufacturer == "" {
				manufacturer = b.Name.Get("en")
			}
		}
	}

	prod.Name = i18n.New(nameAr, nameEn)
	prod.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	prod.ScientificName = r.FormValue("generic_name")
	prod.Active = r.FormValue("active_ingredient")
	prod.DosageForm = r.FormValue("dosage_form")
	prod.BrandID = brandIDPtr
	prod.CategoryID = categoryIDPtr
	prod.ManufacturingCompanies = manufacturer
	prod.SKU = r.FormValue("eda_reg_number")
	prod.Barcode = r.FormValue("eda_reg_number")
	prod.Image = imgURL
	prod.Price = priceVal
	if st := r.FormValue("status"); st != "" {
		prod.Status = catalog.ProductStatus(st)
	}

	if err := h.catSvc.UpdateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin update product failed", "error", err, "id", id)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", i18n.T(lang, "admin.products.updated_success"))
}

// AdminProductDeleteSubmit deletes a master medicine from the catalog.
func (h *UIHandler) AdminProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		if err := h.catSvc.DeleteProduct(database.AsSystem(ctx), id); err != nil {
			h.log.ErrorContext(ctx, "admin delete product failed", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", i18n.T(lang, "admin.products.deleted_success"))
}

// AdminProductsSampleCSV streams a UTF-8 BOM CSV template with sample pharmaceutical products.
func (h *UIHandler) AdminProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.csv\"")

	// Excel on Windows reads a BOM-less UTF-8 CSV as the system codepage and
	// renders every Arabic name as mojibake, so the admin "fixes" it by saving
	// in a codepage the importer then has to guess at. The BOM avoids the whole
	// round trip.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write(importSampleHeaders)
	for _, row := range importSampleRows {
		_ = writer.Write(row)
	}
}

// AdminProductsSampleXLSX streams the Excel (.xlsx) import template.
func (h *UIHandler) AdminProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	write := func(rowIdx int, values []string) {
		for colIdx, value := range values {
			cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx)
			if err != nil {
				continue
			}
			_ = f.SetCellValue(sheet, cell, value)
		}
	}

	write(1, importSampleHeaders)
	for i, row := range importSampleRows {
		write(i+2, row)
	}

	// Right-to-left, so the sheet opens the way an Arabic-speaking admin reads
	// it and column A is where they expect it.
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	// Apply header styling and clean layout
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowHeight(sheet, 1, 26)
	for colIdx := range importSampleHeaders {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}
	colWidths := []float64{28, 28, 16, 16, 26, 26, 16, 14, 12, 22, 14, 14, 16, 24, 24}
	for i, w := range colWidths {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, colName, colName, w)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.xlsx\"")
	if _, err := f.WriteTo(w); err != nil {
		h.log.ErrorContext(r.Context(), "write products sample xlsx", "error", err)
	}
}

func boolPtr(b bool) *bool { return &b }
