package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminInstitutionalPage renders the institutional hierarchy and classification screen.
func (h *UIHandler) AdminInstitutionalPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []*org.InstitutionalWork
	var allWorks []*org.InstitutionalWork
	var unassignedBranches []*org.BranchWithoutWorks
	if h.orgSvc != nil {
		if loaded, err := h.orgSvc.ListInstitutionalWorks(ctx, false); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListInstitutionalWorks", err)
		} else {
			items = loaded
		}
		if loaded, err := h.orgSvc.ListAllFlatInstitutionalWorks(ctx, false); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListAllFlatInstitutionalWorks", err)
		} else {
			allWorks = loaded
		}
		if loaded, err := h.orgSvc.ListBranchesWithoutInstitutionalWorks(ctx); err != nil {
			h.logLoadErr(ctx, "orgSvc.ListBranchesWithoutInstitutionalWorks", err)
		} else {
			unassignedBranches = loaded
		}
	}

	h.renderPage(ctx, w, "render admin institutional", pages.AdminInstitutional(lang, dir, items, allWorks, unassignedBranches))
}

func (h *UIHandler) redirectAdminInstitutional(w http.ResponseWriter, r *http.Request, noticeType, message string) {
	if r.Header.Get("Accept") == "application/json" || r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.Header().Set("Content-Type", "application/json")
		if noticeType == "error" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": message})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": message})
		}
		return
	}

	tab := strings.TrimSpace(r.FormValue("tab"))
	if tab == "" {
		if u, err := url.Parse(r.Referer()); err == nil && u != nil {
			tab = strings.TrimSpace(u.Query().Get("tab"))
		}
	}
	target := "/admin/institutional"
	if tab != "" {
		target += "?tab=" + url.QueryEscape(tab)
	}
	h.redirectWithNotice(w, r, target, noticeType, message)
}

// AdminInstitutionalNewSubmit creates a new institutional work category.
func (h *UIHandler) AdminInstitutionalNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectAdminInstitutional(w, r, "error", i18n.T(lang, "admin.inst.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectAdminInstitutional(w, r, "error", i18n.T(lang, "admin.inst.title_required"))
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
		h.redirectAdminInstitutional(w, r, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectAdminInstitutional(w, r, "success", i18n.T(lang, "admin.inst.created_success"))
}

// AdminInstitutionalEditSubmit updates an existing institutional category.
func (h *UIHandler) AdminInstitutionalEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectAdminInstitutional(w, r, "error", i18n.T(lang, "admin.inst.invalid_id"))
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectAdminInstitutional(w, r, "error", i18n.T(lang, "admin.inst.title_required"))
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
		h.redirectAdminInstitutional(w, r, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectAdminInstitutional(w, r, "success", i18n.T(lang, "admin.inst.updated_success"))
}

// AdminInstitutionalDeleteSubmit soft deletes an institutional category.
func (h *UIHandler) AdminInstitutionalDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.DeleteInstitutionalWork(ctx, id)
	}
	h.redirectAdminInstitutional(w, r, "success", i18n.T(langOf(r), "admin.inst.deleted_success"))
}

// AdminInstitutionalStatusSubmit toggles active status of an institutional category.
func (h *UIHandler) AdminInstitutionalStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ToggleInstitutionalWorkStatus(ctx, id)
	}
	h.redirectAdminInstitutional(w, r, "success", i18n.T(langOf(r), "admin.inst.status_updated_success"))
}
