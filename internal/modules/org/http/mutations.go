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

// UpdateOrg handles organization updates.
func (h *Handler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	if err := orgAccess(r.Context(), id, "organization.update", "org.organization.update"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var o org.Organization
	if err := httpx.DecodeJSON(w, r, &o); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	o.ID = id

	if !actor.IsStaff && !actor.Can("org.admin") {
		existing, err := h.service.GetOrganization(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, h.log, err)
			return
		}
		// Non-staff callers may not alter credit limits, terms, types, or statuses
		o.CreditLimit = existing.CreditLimit
		o.PaymentTermsDays = existing.PaymentTermsDays
		o.Type = existing.Type
		o.Status = existing.Status
		o.MinOrderPrice = existing.MinOrderPrice
		o.MaxOrderPrice = existing.MaxOrderPrice
		o.VerificationNotes = existing.VerificationNotes
		// Registration identity changes go through the reviewed profile change
		// request, not a direct write.
		o.LegalName = existing.LegalName
		o.TaxNumber = existing.TaxNumber
		o.CommercialRegister = existing.CommercialRegister
		o.PharmacistLicense = existing.PharmacistLicense
	}

	if err := h.service.UpdateOrganization(r.Context(), &o); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteOrg handles organization suspension/deletion.
func (h *Handler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	// Deleting an organisation is its owner's decision, or platform staff's.
	if a, ok := authctx.From(r.Context()); !ok || !((a.IsStaff && a.Can("org.organization.delete")) || (a.IsOwner && a.OrganizationID == id)) {
		httpx.Error(w, r, h.log, apperr.Forbidden("org.owner_required", "Only the organization owner can delete it."))
		return
	}

	if err := h.service.DeleteOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// UpdateBranch handles branch updates.
func (h *Handler) UpdateBranch(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("org_id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	if err := orgAccess(r.Context(), orgID, "branch.update", "org.branch.update"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	bid, err := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("bid.invalid", "Invalid branch ID", nil))
		return
	}

	var b org.Branch
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	b.ID = bid
	b.OrganizationID = orgID

	if err := h.service.UpdateBranch(r.Context(), &b); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteBranch removes a branch.
func (h *Handler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("org_id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	if err := orgAccess(r.Context(), orgID, "branch.delete", "org.branch.delete"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	bid, err := strconv.ParseInt(chi.URLParam(r, "bid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("bid.invalid", "Invalid branch ID", nil))
		return
	}

	if err := h.service.DeleteBranch(r.Context(), bid, orgID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// UpdateMemberRole changes a member role.
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("org_id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	if err := orgAccess(r.Context(), orgID, "team.update", "org.member.manage"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	uid, err := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("uid.invalid", "Invalid user ID", nil))
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.UpdateMemberRole(r.Context(), orgID, uid, body.Role); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// RemoveMember removes a user from an organization.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("org_id.invalid", "Invalid organization ID", nil))
		return
	}

	// Authentication established who is calling; it says nothing about which
	// tenant they may act on. Without this, any logged-in user could address any
	// organization by changing the id in the URL.
	if err := orgAccess(r.Context(), orgID, "team.delete", "org.member.manage"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	uid, err := strconv.ParseInt(chi.URLParam(r, "uid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("uid.invalid", "Invalid user ID", nil))
		return
	}

	if err := h.service.RemoveMember(r.Context(), orgID, uid); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
