package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminComponentGalleryPage renders the Phase 3 component matrix gallery for visual regression capture.
func (h *UIHandler) AdminComponentGalleryPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	density := r.URL.Query().Get("density")
	if density == "" {
		density = "comfortable"
	}
	theme := r.URL.Query().Get("theme")
	if theme == "" {
		theme = "light"
	}
	customDir := r.URL.Query().Get("dir")
	if customDir != "" {
		dir = customDir
	}

	props := pages.ComponentGalleryProps{
		Density: density,
		Theme:   theme,
		Lang:    lang,
		Dir:     dir,
	}

	h.renderPage(r.Context(), w, "render component gallery", pages.ComponentGallery(props))
}
