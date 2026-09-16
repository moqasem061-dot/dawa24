package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminRoutes mounts administrative organization routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		// These handlers read and write across every tenant with
		// database.AsSystem. Without this guard the whole group was reachable
		// by any authenticated user, matching neither the other modules nor
		// the intent of an /admin/ path.
		admin.Use(authctx.RequirePermission("org.admin"))

		admin.Get("/api/v1/admin/org/pending", h.ListPendingOrgs)
		admin.Post("/api/v1/admin/org/{id}/approve", h.AdminApproveOrg)
		admin.Post("/api/v1/admin/org/{id}/reject", h.AdminRejectOrg)
		admin.Post("/api/v1/admin/org/{id}/suspend", h.AdminSuspendOrg)
		admin.Put("/api/v1/admin/org/{id}", h.AdminUpdateOrg)
		admin.Get("/api/v1/admin/org/members", h.AdminListAllMembers)
	})
}

// ListPendingOrgs returns organizations awaiting platform approval.
func (h *Handler) ListPendingOrgs(w http.ResponseWriter, r *http.Request) {
	pendingStatus := org.StatusPending
	orgs, err := h.service.ListOrganizations(r.Context(), nil, &pendingStatus, 50, 0)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pending_organizations": orgs, "count": len(orgs)})
}

// AdminApproveOrg approves a pending organization.
func (h *Handler) AdminApproveOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.ApproveOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// AdminRejectOrg rejects an organization application.
func (h *Handler) AdminRejectOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.RejectOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// AdminSuspendOrg suspends an organization.
func (h *Handler) AdminSuspendOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.SuspendOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

// AdminUpdateOrg force-updates an organization.
func (h *Handler) AdminUpdateOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	var o org.Organization
	if err := httpx.DecodeJSON(w, r, &o); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	o.ID = id
	if err := h.service.UpdateOrganization(r.Context(), &o); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

// AdminListAllMembers lists all members across organizations.
func (h *Handler) AdminListAllMembers(w http.ResponseWriter, r *http.Request) {
	// TODO: implement list all members across organizations
	httpx.JSON(w, http.StatusOK, map[string]any{"members": []any{}})
}
