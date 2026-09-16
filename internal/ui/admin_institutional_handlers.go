package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminInstitutionalPage renders the institutional hierarchy and classification screen.
func (h *UIHandler) AdminInstitutionalPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []*org.InstitutionalWork
	var allWorks []*org.InstitutionalWork
	var unassignedBranches []*org.BranchWithoutWorks
	if h.orgSvc != nil {
		items, _ = h.orgSvc.ListInstitutionalWorks(ctx, false)
		allWorks, _ = h.orgSvc.ListAllFlatInstitutionalWorks(ctx, false)
		unassignedBranches, _ = h.orgSvc.ListBranchesWithoutInstitutionalWorks(ctx)
	}

	h.renderPage(ctx, w, "render admin institutional", pages.AdminInstitutional(lang, dir, items, allWorks, unassignedBranches))
}

// AdminInstitutionalNewSubmit creates a new institutional work category.
func (h *UIHandler) AdminInstitutionalNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", i18n.T(lang, "admin.inst.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", i18n.T(lang, "admin.inst.title_required"))
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("parent_id"), 10, 64); err == nil && pid > 0 {
		parentID = &pid
	}

	viewType, _ := strconv.Atoi(r.PostFormValue("view_type"))
	if viewType <= 0 {
		viewType = 1
	}

	icon := strings.TrimSpace(r.PostFormValue("icon"))
	if icon == "" {
		icon = "building"
	}

	pricingType := strings.TrimSpace(r.PostFormValue("pricing_type"))
	if pricingType == "" {
		pricingType = "free"
	}

	isActive := r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	var allowedConnections []int64
	for _, val := range r.PostForm["connections"] {
		if toID, err := strconv.ParseInt(val, 10, 64); err == nil && toID > 0 {
			allowedConnections = append(allowedConnections, toID)
		}
	}

	slug := strings.ToLower(strings.ReplaceAll(titleEn, " ", "-"))
	if slug == "" {
		slug = fmt.Sprintf("work-%d", time.Now().UnixNano()%1000000)
	}

	iw := &org.InstitutionalWork{
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		Icon:               icon,
		PricingType:        org.PricingType(pricingType),
		IsActive:           isActive,
		ViewType:           viewType,
		Slug:               slug,
		ParentID:           parentID,
		AllowedConnections: allowedConnections,
	}

	if err := h.orgSvc.CreateInstitutionalWork(ctx, iw); err != nil {
		h.log.ErrorContext(ctx, "failed to create institutional work", "error", err)
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", i18n.T(lang, "admin.inst.created_success"))
}

// AdminInstitutionalEditSubmit updates an existing institutional category.
func (h *UIHandler) AdminInstitutionalEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", i18n.T(lang, "admin.inst.invalid_id"))
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", i18n.T(lang, "admin.inst.title_required"))
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("parent_id"), 10, 64); err == nil && pid > 0 && pid != id {
		parentID = &pid
	}

	viewType, _ := strconv.Atoi(r.PostFormValue("view_type"))
	if viewType <= 0 {
		viewType = 1
	}

	icon := strings.TrimSpace(r.PostFormValue("icon"))
	if icon == "" {
		icon = "building"
	}

	pricingType := strings.TrimSpace(r.PostFormValue("pricing_type"))
	if pricingType == "" {
		pricingType = "free"
	}

	isActive := r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	var allowedConnections []int64
	for _, val := range r.PostForm["connections"] {
		if toID, err := strconv.ParseInt(val, 10, 64); err == nil && toID > 0 && toID != id {
			allowedConnections = append(allowedConnections, toID)
		}
	}

	iw := &org.InstitutionalWork{
		ID:                 id,
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		Icon:               icon,
		PricingType:        org.PricingType(pricingType),
		IsActive:           isActive,
		ViewType:           viewType,
		ParentID:           parentID,
		AllowedConnections: allowedConnections,
	}

	if err := h.orgSvc.UpdateInstitutionalWork(ctx, iw); err != nil {
		h.log.ErrorContext(ctx, "failed to update institutional work", "error", err)
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", i18n.T(lang, "admin.inst.updated_success"))
}

// AdminInstitutionalDeleteSubmit soft deletes an institutional category.
func (h *UIHandler) AdminInstitutionalDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.DeleteInstitutionalWork(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", i18n.T(langOf(r), "admin.inst.deleted_success"))
}

// AdminInstitutionalStatusSubmit toggles active status of an institutional category.
func (h *UIHandler) AdminInstitutionalStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ToggleInstitutionalWorkStatus(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", i18n.T(langOf(r), "admin.inst.status_updated_success"))
}
