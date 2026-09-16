package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The platform's warehouse administration.
//
// An administrator has no tenant of their own, so every write here runs
// AsSystem against the organisation that owns the row rather than through the
// vendor service's tenant-scoped path — which would refuse, because the
// warehouse belongs to somebody else. The route guard is what establishes that
// the caller is staff; this file assumes it and does the work.

// adminWarehouseReturn is the listing URL to come back to, filters intact.
//
// The action forms carry the filter query string, so approving one warehouse
// does not throw away the page the operator was reading. B1 preserves it on the
// redirect; this is what puts it on the form in the first place.
func adminWarehouseReturn(r *http.Request) string {
	if q := r.URL.RawQuery; q != "" {
		return "/admin/warehouses?" + q
	}
	return "/admin/warehouses"
}

// AdminWarehouseToggleSubmit enables or disables any organisation's warehouse.
func (h *UIHandler) AdminWarehouseToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	back := adminWarehouseReturn(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.invSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}

	active := r.PostFormValue("active") == "1"
	if err := h.invSvc.AdminSetWarehouseActive(database.AsSystem(ctx), id, active); err != nil {
		h.log.ErrorContext(ctx, "toggle warehouse", "warehouse_id", id, "error", err)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "admin.wh.status_changed"))
}

// AdminWarehouseEditSubmit updates any organisation's warehouse.
func (h *UIHandler) AdminWarehouseEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	back := adminWarehouseReturn(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.invSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}

	// The owning organisation is read from the stored row, never from the form.
	// Accepting it from a request body would let an administrator move a
	// warehouse — and the stock inside it — into another tenant.
	sysCtx := database.AsSystem(ctx)
	existing, err := h.invSvc.GetWarehouse(sysCtx, id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}

	existing.Name = name
	existing.Code = strings.TrimSpace(r.PostFormValue("code"))
	existing.Phone = strings.TrimSpace(r.PostFormValue("phone"))
	existing.Address = strings.TrimSpace(r.PostFormValue("address"))
	existing.IsActive = r.PostFormValue("is_active") == "1"

	if err := h.invSvc.AdminUpdateWarehouse(sysCtx, existing); err != nil {
		h.log.ErrorContext(ctx, "admin update warehouse", "warehouse_id", id, "error", err)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "admin.wh.saved"))
}

// AdminWarehouseCreateSubmit creates a warehouse on behalf of an organisation.
func (h *UIHandler) AdminWarehouseCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	back := adminWarehouseReturn(r)

	if h.invSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}
	orgID, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("organization_id")), 10, 64)
	name := strings.TrimSpace(r.PostFormValue("name"))
	if orgID <= 0 || name == "" {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.wh.invalid"))
		return
	}

	wh := &inventory.Warehouse{
		OrganizationID: orgID,
		Name:           name,
		Code:           strings.TrimSpace(r.PostFormValue("code")),
		Phone:          strings.TrimSpace(r.PostFormValue("phone")),
		Address:        strings.TrimSpace(r.PostFormValue("address")),
		IsActive:       r.PostFormValue("is_active") == "1",
	}
	if err := h.invSvc.AdminCreateWarehouse(database.AsSystem(ctx), wh); err != nil {
		h.log.ErrorContext(ctx, "admin create warehouse", "org_id", orgID, "error", err)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "admin.wh.saved"))
}

// adminWarehouseData reads the listing's filters off the query string.
func adminWarehouseData(r *http.Request) pages.AdminWarehousesData {
	q := r.URL.Query()
	d := pages.AdminWarehousesData{
		SearchQuery:  strings.TrimSpace(q.Get("q")),
		ActiveFilter: strings.TrimSpace(q.Get("active")),
	}
	if d.ActiveFilter != "1" && d.ActiveFilter != "0" {
		d.ActiveFilter = ""
	}
	d.OrganizationID = parseIDParam(q.Get("org_id"))
	return d
}

// adminWarehouseFilter converts the page's filter state into the repository's.
func adminWarehouseFilter(d pages.AdminWarehousesData, limit, offset int) inventory.AdminWarehouseFilter {
	f := inventory.AdminWarehouseFilter{
		Query:          d.SearchQuery,
		OrganizationID: d.OrganizationID,
		Limit:          limit,
		Offset:         offset,
	}
	switch d.ActiveFilter {
	case "1":
		yes := true
		f.Active = &yes
	case "0":
		no := false
		f.Active = &no
	}
	return f
}
