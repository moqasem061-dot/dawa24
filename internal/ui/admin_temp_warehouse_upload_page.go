package ui

import (
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminTempWarehouseUploadPage renders the dedicated standalone upload page for temporary warehouses.
func (h *UIHandler) AdminTempWarehouseUploadPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	path := r.URL.Path
	pageURL := tempWarehouseSuperPage
	base := tempWarehouseSuperBase
	teamView := false
	mineOnly := false

	if strings.HasPrefix(path, "/admin/my/temparte-warehouses") {
		pageURL = tempWarehouseMineBase
		base = tempWarehouseMineBase
		mineOnly = true
	} else if strings.HasPrefix(path, "/admin/team/temparte-warehouses") {
		pageURL = tempWarehouseTeamBase
		base = tempWarehouseTeamBase
		teamView = true
	}

	data := &pages.AdminTempWarehousesData{
		Base:     base,
		PageURL:  pageURL,
		MineOnly: mineOnly,
		TeamView: teamView,
	}

	h.renderPage(ctx, w, "render temp warehouse upload page", pages.AdminTempWarehouseUploadPage(data, lang, dir))
}
