package ui

import (
	"fmt"
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

// AdminCategoriesPage renders master product categories CRUD.
func (h *UIHandler) AdminCategoriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	sysCtx := database.AsSystem(ctx)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	page := pagination.PageNumber(r)
	pageSize := pagination.RowsPerPage(r)
	offset := (page - 1) * pageSize

	var allCats []*catalog.Category
	var items []pages.CategoryViewItem
	var totalCount int

	if h.catSvc != nil {
		allCats, _ = h.catSvc.ListCategories(sysCtx)
		catCounts, total, err := h.catSvc.ListCategoriesWithProductCount(sysCtx, search, status, pageSize, offset)
		if err == nil {
			totalCount = total
			for _, cc := range catCounts {
				if cc != nil && cc.Category != nil {
					items = append(items, pages.CategoryViewItem{
						Category:     cc.Category,
						ProductCount: cc.ProductCount,
					})
				}
			}
		}
	}

	h.renderPage(ctx, w, "render admin categories", pages.AdminCategoriesPage(items, allCats, totalCount, page, pageSize, search, status, lang, dir))
}

// AdminCategoryCreateSubmit creates a new product category.
func (h *UIHandler) AdminCategoryCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	var parentIDPtr *int64
	if parentIDStr := strings.TrimSpace(r.FormValue("parent_id")); parentIDStr != "" {
		if pid, err := strconv.ParseInt(parentIDStr, 10, 64); err == nil && pid > 0 {
			parentIDPtr = &pid
		}
	}

	cat := &catalog.Category{
		ParentID:    parentIDPtr,
		Name:        i18n.New(nameAr, nameEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		SortOrder:   sortOrder,
		Status:      status,
	}

	if _, err := h.catSvc.CreateCategory(database.AsSystem(ctx), cat); err != nil {
		h.log.ErrorContext(ctx, "admin create category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", i18n.T(lang, "admin.categories.created_success"))
}

// AdminCategoryEditSubmit updates an existing product category.
func (h *UIHandler) AdminCategoryEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || catID <= 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.invalid_id"))
		return
	}

	_ = r.ParseForm()

	cat, err := h.catSvc.GetCategory(database.AsSystem(ctx), catID)
	if err != nil || cat == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.not_found"))
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.name_required"))
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = cat.Status
	}

	var parentIDPtr *int64
	if parentIDStr := strings.TrimSpace(r.FormValue("parent_id")); parentIDStr != "" {
		if pid, err := strconv.ParseInt(parentIDStr, 10, 64); err == nil && pid > 0 {
			if pid != catID { // category cannot be its own parent
				parentIDPtr = &pid
			}
		}
	}

	cat.ParentID = parentIDPtr
	cat.Name = i18n.New(nameAr, nameEn)
	cat.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	cat.SortOrder = sortOrder
	cat.Status = status

	if err := h.catSvc.UpdateCategory(database.AsSystem(ctx), cat); err != nil {
		h.log.ErrorContext(ctx, "admin update category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", i18n.T(lang, "admin.categories.updated_success"))
}

// AdminCategoryToggleSubmit toggles a category's active status.
func (h *UIHandler) AdminCategoryToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err == nil && catID > 0 && h.catSvc != nil {
		cat, err := h.catSvc.GetCategory(database.AsSystem(ctx), catID)
		if err == nil && cat != nil {
			newStatus := "inactive"
			if cat.Status == "inactive" {
				newStatus = "active"
			}
			cat.Status = newStatus
			_ = h.catSvc.UpdateCategory(database.AsSystem(ctx), cat)
		}
	}
	h.redirectWithNotice(w, r, "/admin/categories", "success", i18n.T(lang, "admin.categories.status_updated_success"))
}

// AdminCategoryDeleteSubmit deletes a category if no products are linked.
func (h *UIHandler) AdminCategoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || catID <= 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", i18n.T(lang, "admin.categories.invalid_id"))
		return
	}

	count, _ := h.catSvc.CountProductsInCategory(database.AsSystem(ctx), catID)
	if count > 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", fmt.Sprintf(i18n.T(lang, "admin.categories.cannot_delete_has_products_format"), count))
		return
	}

	if err := h.catSvc.DeleteCategory(database.AsSystem(ctx), catID); err != nil {
		h.log.WarnContext(ctx, "admin delete category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", i18n.T(lang, "admin.categories.deleted_success"))
}

// AdminSocialMediaPage renders social media channel links.
func (h *UIHandler) AdminSocialMediaPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		if ss, err := h.adminSvc.GetSiteSettings(ctx); err == nil && ss != nil {
			idx := int64(1)
			for platform, url := range ss.SocialLinks {
				items = append(items, pages.ReferenceItem{
					ID:          idx,
					Name:        platform,
					Description: url,
					Status:      "active",
					Extra:       platform,
				})
				idx++
			}
		}
	}
	h.renderPage(ctx, w, "render admin social media", pages.AdminReferenceCRUDPage(i18n.T(lang, "admin.reference.social_media_title"), "social-media", i18n.T(lang, "admin.reference.social_channel"), items, "settings", lang, dir))
}

// AdminApiIntegrationsPage renders third-party API configurations.
func (h *UIHandler) AdminApiIntegrationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		if gw, err := h.adminSvc.GetGatewaySettings(ctx); err == nil && gw != nil {
			gwStatus := "inactive"
			if gw.IsActive {
				gwStatus = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          1,
				Name:        i18n.T(lang, "admin.reference.api_gateway_title"),
				Description: fmt.Sprintf(i18n.T(lang, "admin.reference.api_gateway_desc_format"), gw.Environment, gw.EndpointURL),
				Status:      gwStatus,
				Extra:       fmt.Sprintf(i18n.T(lang, "admin.reference.api_gateway_timeout_format"), gw.TimeoutSeconds),
			})
		}
		if ai, err := h.adminSvc.GetAISettings(ctx); err == nil && ai != nil {
			aiStatus := "inactive"
			if ai.IsActive {
				aiStatus = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          2,
				Name:        i18n.T(lang, "admin.reference.ai_gateway_title"),
				Description: fmt.Sprintf(i18n.T(lang, "admin.reference.ai_gateway_desc_format"), ai.EndpointURL),
				Status:      aiStatus,
				Extra:       fmt.Sprintf(i18n.T(lang, "admin.reference.ai_gateway_max_tokens_format"), ai.MaxTokens),
			})
		}
	}
	h.renderPage(ctx, w, "render admin api integrations", pages.AdminReferenceCRUDPage(i18n.T(lang, "admin.reference.api_integrations_title"), "api-integrations", i18n.T(lang, "admin.reference.api_integration"), items, "developers", lang, dir))
}
