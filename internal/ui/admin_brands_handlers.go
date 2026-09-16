package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminBrandsPage renders the master pharmaceutical brands and manufacturers management console.
func (h *UIHandler) AdminBrandsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	sysCtx := database.AsSystem(ctx)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var brandItems []pages.BrandViewItem
	var totalCount int
	if h.catSvc != nil {
		brandsWithCount, total, err := h.catSvc.ListBrandsWithProductCount(sysCtx, search, status, limit, offset)
		if err == nil {
			totalCount = total
			for _, item := range brandsWithCount {
				brandItems = append(brandItems, pages.BrandViewItem{
					Brand:        item.Brand,
					ProductCount: item.ProductCount,
				})
			}
		}
	}

	h.renderPage(ctx, w, "render admin brands", pages.AdminBrandsPage(brandItems, totalCount, page, limit, search, status, lang, dir))
}

// AdminBrandCreateSubmit creates a new pharmaceutical brand / manufacturer in the database.
func (h *UIHandler) AdminBrandCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.brands.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "brand_image", "brands")
	if imgURL == "" {
		imgURL = strings.TrimSpace(r.FormValue("image_url"))
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	brand := &catalog.Brand{
		Name:        i18n.New(nameAr, nameEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		Image:       imgURL,
		Status:      status,
	}

	if _, err := h.catSvc.CreateBrand(database.AsSystem(ctx), brand); err != nil {
		h.log.ErrorContext(ctx, "admin create brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", i18n.T(lang, "admin.brands.created_success"))
}

// AdminBrandEditSubmit updates an existing brand / manufacturer in the database.
func (h *UIHandler) AdminBrandEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || brandID <= 0 {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.brands.invalid_id"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	brand, err := h.catSvc.GetBrand(database.AsSystem(ctx), brandID)
	if err != nil || brand == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.brands.not_found"))
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.brands.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "brand_image", "brands")
	if imgURL == "" {
		imgURL = strings.TrimSpace(r.FormValue("image_url"))
	}
	if imgURL == "" {
		imgURL = brand.Image
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = brand.Status
	}

	brand.Name = i18n.New(nameAr, nameEn)
	brand.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	brand.Image = imgURL
	brand.Status = status

	if err := h.catSvc.UpdateBrand(database.AsSystem(ctx), brand); err != nil {
		h.log.ErrorContext(ctx, "admin update brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", i18n.T(lang, "admin.brands.updated_success"))
}

// AdminBrandStatusSubmit toggles a brand's active status.
func (h *UIHandler) AdminBrandStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err == nil && brandID > 0 && h.catSvc != nil {
		brand, err := h.catSvc.GetBrand(database.AsSystem(ctx), brandID)
		if err == nil && brand != nil {
			newStatus := r.FormValue("status")
			if newStatus == "" {
				if brand.Status == "active" {
					newStatus = "inactive"
				} else {
					newStatus = "active"
				}
			}
			brand.Status = newStatus
			_ = h.catSvc.UpdateBrand(database.AsSystem(ctx), brand)
		}
	}
	http.Redirect(w, r, "/admin/brands", http.StatusSeeOther)
}

// AdminBrandDeleteSubmit deletes a brand if no products are linked to it.
func (h *UIHandler) AdminBrandDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || brandID <= 0 {
		h.redirectWithNotice(w, r, "/admin/brands", "error", i18n.T(lang, "admin.brands.invalid_id"))
		return
	}

	if err := h.catSvc.DeleteBrand(database.AsSystem(ctx), brandID); err != nil {
		h.log.WarnContext(ctx, "admin delete brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", i18n.T(lang, "admin.brands.deleted_success"))
}
