package ui

import (
	"context"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AdminProductImagesSyncSubmit triggers background download and local volume storage
// of all missing or remote-only product images across the master catalog.
func (h *UIHandler) AdminProductImagesSyncSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	lang := langOf(r)

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error",
			i18n.T(lang, "admin.categories.service_unavailable"))
		return
	}

	h.safeGo("admin-sync-catalog-images", func() {
		bgCtx := database.AsSystem(context.Background())
		count, err := h.BackfillMissingProductImages(bgCtx, 25000)
		if err != nil {
			h.log.ErrorContext(bgCtx, "backfill missing product images failed", "error", err)
			return
		}
		h.log.InfoContext(bgCtx, "backfill missing product images queued", "queued_targets", count)
	})

	notice := "تم بدء عملية تنزيل وتثبيت صور الكتالوج على السيرفر في الخلفية بنجاح"
	if lang == "en" {
		notice = "Catalog image sync started in the background successfully"
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", notice)
}
