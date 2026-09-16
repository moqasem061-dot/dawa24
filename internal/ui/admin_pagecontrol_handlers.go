package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

const pageControlBase = "/admin/system-pages"

// AdminSystemPagesPage lists every managed route root and its enable/disable
// state, with search filtering and rows-per-page pagination.
func (h *UIHandler) AdminSystemPagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)

	if h.pageControl == nil {
		h.renderPage(ctx, w, "render system pages", pages.AdminSystemPagesPage(pages.SystemPagesView{
			NoticeKind: "error", Notice: i18n.T(lang, "admin.pagecontrol.service_unavailable"),
		}, lang, dir))
		return
	}

	all, err := h.pageControl.List(ctx, pagecontrol.Filter{})
	if err != nil {
		h.log.ErrorContext(ctx, "list managed pages", "error", err)
	}

	filter := strings.TrimSpace(r.URL.Query().Get("resource"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	lowerQ := strings.ToLower(q)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	if limit <= 0 {
		limit = 25
	}

	counts := make(map[string]int)
	var matchedRows []pages.SystemPageRow

	for _, p := range all {
		counts[string(p.Resource)]++

		if filter != "" && string(p.Resource) != filter {
			continue
		}

		if statusFilter == "active" && !p.IsEnabled {
			continue
		}
		if statusFilter == "inactive" && p.IsEnabled {
			continue
		}

		label := p.Label(lang)
		if q != "" {
			matchPath := strings.Contains(strings.ToLower(p.Path), lowerQ)
			matchLabel := strings.Contains(strings.ToLower(label), lowerQ)
			matchDesc := strings.Contains(strings.ToLower(p.Description), lowerQ)
			matchLabelAr := strings.Contains(strings.ToLower(p.LabelAr), lowerQ)
			matchLabelEn := strings.Contains(strings.ToLower(p.LabelEn), lowerQ)
			if !matchPath && !matchLabel && !matchDesc && !matchLabelAr && !matchLabelEn {
				continue
			}
		}

		matchedRows = append(matchedRows, pages.SystemPageRow{
			ID:           p.ID,
			Label:        label,
			LabelAr:      p.LabelAr,
			LabelEn:      p.LabelEn,
			Description:  p.Description,
			Path:         p.Path,
			MatchMode:    string(p.MatchMode),
			Resource:     string(p.Resource),
			Enabled:      p.IsEnabled,
			IsSystem:     p.IsSystem,
			IsLockable:   p.IsLockable,
			Source:       string(p.Source),
			PatternCount: len(p.RoutePatterns),
			UpdatedAt:    p.UpdatedAt.Format("2006-01-02 15:04"),
		})
	}

	totalFiltered := len(matchedRows)
	var pagedRows []pages.SystemPageRow
	if limit > 0 {
		start := (page - 1) * limit
		if start < totalFiltered {
			end := start + limit
			if end > totalFiltered {
				end = totalFiltered
			}
			pagedRows = matchedRows[start:end]
		}
	} else {
		pagedRows = matchedRows
	}

	qVals := url.Values{}
	if filter != "" {
		qVals.Set("resource", filter)
	}
	if statusFilter != "" {
		qVals.Set("status", statusFilter)
	}
	if q != "" {
		qVals.Set("q", q)
	}

	view := pages.SystemPagesView{
		Rows:          pagedRows,
		Filter:        filter,
		StatusFilter:  statusFilter,
		SearchQuery:   q,
		Counts:        counts,
		Total:         len(all),
		TotalFiltered: totalFiltered,
		CanCreate:     actor.Can("platform.page_control.create"),
		CanUpdate:     actor.Can("platform.page_control.update"),
		CanDelete:     actor.Can("platform.page_control.delete"),
		NoticeKind:    r.URL.Query().Get("notice"),
		Notice:        r.URL.Query().Get("msg"),
		Pagination: components.PaginationProps{
			CurrentPage:     page,
			PageSize:        limit,
			TotalCount:      totalFiltered,
			BaseURL:         pageControlBase,
			QueryValues:     qVals,
			PageSizeOptions: []int{10, 25, 50, 100},
		},
	}

	h.renderPage(ctx, w, "render system pages", pages.AdminSystemPagesPage(view, lang, dir))
}

// AdminSystemPageEditSubmit updates an existing managed page.
func (h *UIHandler) AdminSystemPageEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.pageControl == nil {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.service_unavailable"))
		return
	}
	id, ok := pageControlID(r)
	if !ok {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.not_found"))
		return
	}
	_ = r.ParseForm()

	in := pagecontrol.UpdateInput{
		Path:        r.PostFormValue("path"),
		MatchMode:   pagecontrol.MatchMode(strings.TrimSpace(r.PostFormValue("match_mode"))),
		Resource:    pagecontrol.Resource(strings.TrimSpace(r.PostFormValue("resource"))),
		LabelAr:     r.PostFormValue("label_ar"),
		LabelEn:     r.PostFormValue("label_en"),
		Description: r.PostFormValue("description"),
	}
	if _, err := h.pageControl.Update(ctx, id, in, h.pageControlActor(r)); err != nil {
		h.log.WarnContext(ctx, "update managed page", "id", id, "error", err)
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.update_failed"))
		return
	}
	h.reloadPageControl(ctx)
	h.redirectWithNotice(w, r, pageControlBase, "success", i18n.T(lang, "admin.pagecontrol.updated"))
}

// AdminSystemPageToggleSubmit enables or disables one managed page.
func (h *UIHandler) AdminSystemPageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.pageControl == nil {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.service_unavailable"))
		return
	}
	id, ok := pageControlID(r)
	if !ok {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.not_found"))
		return
	}
	enabled := r.PostFormValue("enabled") == "true"

	if _, err := h.pageControl.SetEnabled(ctx, id, enabled, h.pageControlActor(r)); err != nil {
		h.log.WarnContext(ctx, "toggle managed page", "id", id, "enabled", enabled, "error", err)
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.toggle_failed"))
		return
	}
	h.reloadPageControl(ctx)

	msg := i18n.T(lang, "admin.pagecontrol.toggled_disabled")
	if enabled {
		msg = i18n.T(lang, "admin.pagecontrol.toggled_enabled")
	}
	h.redirectWithNotice(w, r, pageControlBase, "success", msg)
}

// AdminSystemPageCreateSubmit adds a manual page to the catalogue.
func (h *UIHandler) AdminSystemPageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.pageControl == nil {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.service_unavailable"))
		return
	}
	_ = r.ParseForm()

	in := pagecontrol.CreateInput{
		Path:        r.PostFormValue("path"),
		MatchMode:   pagecontrol.MatchMode(strings.TrimSpace(r.PostFormValue("match_mode"))),
		Resource:    pagecontrol.Resource(strings.TrimSpace(r.PostFormValue("resource"))),
		LabelAr:     r.PostFormValue("label_ar"),
		LabelEn:     r.PostFormValue("label_en"),
		Description: r.PostFormValue("description"),
	}
	if _, err := h.pageControl.Create(ctx, in, h.pageControlActor(r)); err != nil {
		h.log.WarnContext(ctx, "create managed page", "path", in.Path, "error", err)
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.create_failed"))
		return
	}
	h.reloadPageControl(ctx)
	h.redirectWithNotice(w, r, pageControlBase, "success", i18n.T(lang, "admin.pagecontrol.created"))
}

// AdminSystemPageDeleteSubmit removes a manual page.
func (h *UIHandler) AdminSystemPageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.pageControl == nil {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.service_unavailable"))
		return
	}
	id, ok := pageControlID(r)
	if !ok {
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.not_found"))
		return
	}
	if err := h.pageControl.Delete(ctx, id, h.pageControlActor(r)); err != nil {
		h.log.WarnContext(ctx, "delete managed page", "id", id, "error", err)
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.delete_failed"))
		return
	}
	h.reloadPageControl(ctx)
	h.redirectWithNotice(w, r, pageControlBase, "success", i18n.T(lang, "admin.pagecontrol.deleted"))
}

// AdminSystemPageRescanSubmit re-runs route discovery against the live router.
func (h *UIHandler) AdminSystemPageRescanSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	added, err := pagecontrol.Rescan(ctx)
	if err != nil {
		h.log.WarnContext(ctx, "rescan managed pages", "error", err)
		h.redirectWithNotice(w, r, pageControlBase, "error", i18n.T(lang, "admin.pagecontrol.service_unavailable"))
		return
	}
	h.redirectWithNotice(w, r, pageControlBase, "success", fmt.Sprintf(i18n.T(lang, "admin.pagecontrol.rescanned"), added))
}

// --- helpers ---

func pageControlID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (h *UIHandler) pageControlActor(r *http.Request) pagecontrol.Actor {
	actor := authctx.FromContext(r.Context())
	return pagecontrol.Actor{UserID: actor.UserID, RequestID: pagecontrol.RequestIDFrom(r.Context())}
}

func (h *UIHandler) reloadPageControl(ctx context.Context) {
	if e := pagecontrol.Global(); e != nil {
		_ = e.Reload(ctx)
	}
}
