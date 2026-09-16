package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// dashboardLanding picks the first screen a member of a company can actually
// open.
//
// The dashboard was hardcoded as everyone's landing page, which was true while
// every company role held vendor.dashboard.view. A مندوب holds only إدارة
// الشحنات, so signing in used to drop them on a page their own permissions
// refuse — a 404 as a welcome screen. Walking the sidebar the shell would
// render them and taking its first real link is the general answer: it stays
// correct for any narrow role added later, and for the ordinary member it
// still resolves to the dashboard, because that is the first item in the nav.
func dashboardLanding(scope rbac.Scope, perms []string, fallback string) string {
	held := rbac.NewSet(perms)
	if held.Has(string(scope) + ".dashboard.view") {
		return fallback
	}
	for _, sec := range rbac.VisibleNav(scope, held) {
		for _, item := range sec.Items {
			// Account settings is visible to everyone and is about the caller
			// rather than the company, so it is never a landing page.
			if item.AlwaysVisible || item.Href == "" {
				continue
			}
			return item.Href
		}
	}
	return fallback
}

func (h *UIHandler) findNearestCityID(ctx context.Context, lat, lon float64) int64 {
	cities := h.listCities(ctx)
	if len(cities) == 0 {
		return 1
	}
	var bestID int64 = cities[0].ID
	var minDist float64 = 1e9
	for _, c := range cities {
		if c.Latitude == 0 && c.Longitude == 0 {
			continue
		}
		dLat := lat - c.Latitude
		dLon := lon - c.Longitude
		dist := dLat*dLat + dLon*dLon
		if dist < minDist {
			minDist = dist
			bestID = c.ID
		}
	}
	return bestID
}
