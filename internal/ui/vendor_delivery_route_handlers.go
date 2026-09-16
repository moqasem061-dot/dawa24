package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// خط سير التوصيل — the courier's round drawn as a route, at
// /vendor/delivery/route.
//
// The board answers "what am I carrying". This answers "where do I go now, and
// in what order after that", which is the question a person standing beside a
// motorbike with six parcels actually has. The ordering is computed in
// commerce.BuildCourierRoute; everything here is about where the journey
// starts, because that is the only input the domain cannot work out for
// itself.
//
// Two starting points, in order of preference:
//
//  1. The courier's live position, sent by the browser's Geolocation API as
//     ?lat=&lon=. It is the truthful one: a courier halfway through a round is
//     not at the warehouse, and re-planning from where they actually are is
//     what makes the second half of the day shorter than the first.
//  2. The supplier's warehouse — the branch the courier is bound to, else the
//     main branch. It is what the first plan of the morning is built from,
//     before anyone has granted location permission.
//
// With neither, the plan is still rendered: the stops are listed oldest-parcel
// first and the page says the order is not a route. A courier with no map is
// worse off than one with an unordered list, and much worse off than one with
// an empty screen.

// VendorDeliveryRoutePage renders the planned round.
func (h *UIHandler) VendorDeliveryRoutePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery/route", http.StatusSeeOther)
		return
	}
	if h.commSvc == nil {
		h.renderPage(ctx, w, "render delivery route fallback",
			pages.VendorDeliveryRoutePage(pages.VendorDeliveryRouteData{}, lang, dir))
		return
	}

	origin, live := h.routeOrigin(ctx, r, actor)
	route, err := h.commSvc.PlanCourierRoute(ctx, actor.OrganizationID, actor.UserID, origin)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	route.OriginIsLive = live

	counts, err := h.commSvc.CourierQueueCounts(ctx, actor.OrganizationID, actor.UserID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	noticeType, noticeMsg := noticeFrom(r)
	h.renderPage(ctx, w, "render vendor delivery route", pages.VendorDeliveryRoutePage(
		pages.VendorDeliveryRouteData{
			Route:         route,
			Counts:        counts,
			WarehouseName: h.routeWarehouseName(ctx, actor, lang),
			NoticeType:    noticeType,
			NoticeMsg:     noticeMsg,
		}, lang, dir))
}

// VendorDeliveryRouteData re-plans the round from a position the browser has
// just measured, and returns the map's data without re-rendering the page.
//
// It exists so the map can follow the courier. Reloading the whole document
// every time a GPS fix moves is unusable on the connection these users have;
// this is one small JSON body carrying the stop order, the legs and the pins.
func (h *UIHandler) VendorDeliveryRouteData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 || h.commSvc == nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	origin, live := h.routeOrigin(ctx, r, actor)
	route, err := h.commSvc.PlanCourierRoute(ctx, actor.OrganizationID, actor.UserID, origin)
	if err != nil {
		h.log.WarnContext(ctx, "delivery route: replan failed", "error", err, "user_id", actor.UserID)
		http.Error(w, `{"error":"plan_failed"}`, http.StatusInternalServerError)
		return
	}
	route.OriginIsLive = live

	lang := langOf(r)
	payload := pages.RouteJSON(route, lang)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(ctx, "delivery route: encode payload", "error", err)
	}
}

// routeOrigin resolves where the round starts, and whether that is a live fix.
func (h *UIHandler) routeOrigin(ctx context.Context, r *http.Request, actor authctx.Actor) (commerce.GeoPoint, bool) {
	if p, ok := geoFromQuery(r); ok {
		return p, true
	}
	return h.warehouseOrigin(ctx, actor), false
}

// geoFromQuery reads a browser-supplied fix. A malformed or out-of-range pair
// is treated as absent rather than rejected: the request still has a useful
// answer without it, and refusing the page because a query string was wrong
// would strand the courier.
func geoFromQuery(r *http.Request) (commerce.GeoPoint, bool) {
	lat, errLat := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lat")), 64)
	lon, errLon := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lon")), 64)
	if errLat != nil || errLon != nil {
		return commerce.GeoPoint{}, false
	}
	p := commerce.GeoPoint{Lat: lat, Lon: lon}
	return p, p.IsSet()
}

// warehouseOrigin is the supplier location the courier collects from: the
// branch they are bound to when membership names one, otherwise the company's
// main branch, otherwise the first branch that has coordinates at all.
func (h *UIHandler) warehouseOrigin(ctx context.Context, actor authctx.Actor) commerce.GeoPoint {
	branch := h.courierWarehouse(ctx, actor)
	if branch == nil || branch.Latitude == nil || branch.Longitude == nil {
		return commerce.GeoPoint{}
	}
	p := commerce.GeoPoint{Lat: *branch.Latitude, Lon: *branch.Longitude}
	if !p.IsSet() {
		return commerce.GeoPoint{}
	}
	return p
}

// routeWarehouseName is what the origin pin is called on the map.
func (h *UIHandler) routeWarehouseName(ctx context.Context, actor authctx.Actor, lang string) string {
	if b := h.courierWarehouse(ctx, actor); b != nil {
		return b.Name.Get(i18n.ParseLang(lang))
	}
	return ""
}

// courierWarehouse picks the branch a courier starts their day at.
func (h *UIHandler) courierWarehouse(ctx context.Context, actor authctx.Actor) *org.Branch {
	if h.orgSvc == nil || actor.OrganizationID <= 0 {
		return nil
	}
	branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil || len(branches) == 0 {
		if err != nil {
			h.log.WarnContext(ctx, "delivery route: list branches", "error", err,
				"organization_id", actor.OrganizationID)
		}
		return nil
	}

	var main, anyLocated *org.Branch
	for _, b := range branches {
		if b == nil || b.Latitude == nil || b.Longitude == nil {
			continue
		}
		if actor.BranchID != nil && b.ID == *actor.BranchID {
			return b
		}
		if b.IsMain && main == nil {
			main = b
		}
		if anyLocated == nil {
			anyLocated = b
		}
	}
	if main != nil {
		return main
	}
	return anyLocated
}
