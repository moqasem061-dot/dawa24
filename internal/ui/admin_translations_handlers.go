package ui

import (
	"net/http"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminTranslationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	ns := strings.TrimSpace(r.URL.Query().Get("ns"))
	customStr := strings.TrimSpace(r.URL.Query().Get("custom"))

	var customFilter *bool
	if customStr == "true" {
		t := true
		customFilter = &t
	} else if customStr == "false" {
		f := false
		customFilter = &f
	}

	filter := platformadmin.TranslationFilter{
		Query:     q,
		Namespace: ns,
		Custom:    customFilter,
		Limit:     limit,
		Offset:    offset,
	}

	var translations []*platformadmin.Translation
	var total int
	var err error

	if h.adminSvc != nil {
		translations, total, err = h.adminSvc.ListTranslations(ctx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "admin translations: list failed", "error", err)
		}
	}

	// Fallback to in-memory entries if database table is not yet seeded
	if len(translations) == 0 && q == "" && ns == "" && customFilter == nil {
		entries := i18n.GetAllKeyEntries()
		total = len(entries)
		end := min(offset+limit, total)
		if offset < total {
			for _, e := range entries[offset:end] {
				translations = append(translations, &platformadmin.Translation{
					Key:         e.Key,
					Namespace:   e.Namespace,
					TextAR:      e.TextAR,
					TextEN:      e.TextEN,
					Description: e.Description,
					IsCustom:    e.IsCustom,
				})
			}
		}
	}

	var stats *platformadmin.TranslationStats
	if h.adminSvc != nil {
		stats, _ = h.adminSvc.GetTranslationStats(ctx)
	}
	if stats == nil || stats.TotalKeys == 0 {
		allEntries := i18n.GetAllKeyEntries()
		stats = &platformadmin.TranslationStats{
			TotalKeys:       len(allEntries),
			TotalNamespaces: len(i18n.GetNamespaces()),
		}
	}

	pagProps := components.PaginationProps{
		CurrentPage: page,
		PageSize:    limit,
		TotalCount:  total,
		BaseURL:     r.URL.Path,
		QueryValues: r.URL.Query(),
	}

	data := pages.AdminTranslationsData{
		Translations: translations,
		Stats:        stats,
		Namespaces:   i18n.GetNamespaces(),
		SelectedNS:   ns,
		Query:        q,
		CustomFilter: customStr,
		Pagination:   pagProps,
	}

	_ = pages.AdminTranslations(data, lang, dir).Render(ctx, w)
}

func (h *UIHandler) AdminTranslationUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()

	key := strings.TrimSpace(r.PostFormValue("key"))
	textAR := strings.TrimSpace(r.PostFormValue("text_ar"))
	textEN := strings.TrimSpace(r.PostFormValue("text_en"))
	desc := strings.TrimSpace(r.PostFormValue("description"))

	if key == "" || (textAR == "" && textEN == "") {
		h.redirectWithNotice(w, r, "/admin/translations", "error", i18n.T(lang, "admin.translations.key_and_text_required"))
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/translations", "error", i18n.T(lang, "admin.translations.admin_service_unavailable"))
		return
	}

	if err := h.adminSvc.UpdateTranslation(ctx, key, textAR, textEN, desc); err != nil {
		h.log.ErrorContext(ctx, "admin update translation failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/translations", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/translations", "success", i18n.T(lang, "admin.translations.updated_success"))
}

func (h *UIHandler) AdminTranslationResetSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()

	key := strings.TrimSpace(r.PostFormValue("key"))
	if key == "" {
		h.redirectWithNotice(w, r, "/admin/translations", "error", i18n.T(lang, "admin.translations.invalid_key"))
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/translations", "error", i18n.T(lang, "admin.translations.admin_service_unavailable"))
		return
	}

	if err := h.adminSvc.ResetTranslation(ctx, key); err != nil {
		h.log.ErrorContext(ctx, "admin reset translation failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/translations", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/translations", "success", i18n.T(lang, "admin.translations.reset_success"))
}

func (h *UIHandler) AdminTranslationsSyncSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/translations", "error", i18n.T(lang, "admin.translations.admin_service_unavailable"))
		return
	}

	if err := h.adminSvc.SyncAllDefaultTranslations(ctx); err != nil {
		h.log.ErrorContext(ctx, "admin sync translations failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/translations", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/translations", "success", i18n.T(lang, "admin.translations.synced_success"))
}
