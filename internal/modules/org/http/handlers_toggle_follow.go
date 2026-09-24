package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// ToggleFollow toggles follow.
func (h *Handler) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	following, err := h.service.ToggleFollow(r.Context(), orgID, body.UserID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"following": following})
}

// orgAccess admits a member of the organisation who holds the dashboard key for
// the action ("branch.update" is pharmacy.branch.update or
// vendor.branch.update), or platform staff holding staffKey.
//
// Being in the same organisation is not enough. The JSON routes checked only
// that, so any employee could delete branches, change member roles (their own
// included) or delete the organisation, none of which their dashboard allows.
func orgAccess(ctx context.Context, orgID int64, action, staffKey string) error {
	a, ok := authctx.From(ctx)
	if !ok {
		return apperr.Unauthorized()
	}
	if a.IsStaff {
		if staffKey != "" && a.Can(staffKey) {
			return nil
		}
		return apperr.Forbidden("org.permission_required", "You do not have permission for this organization action.")
	}
	if a.OrganizationID <= 0 || a.OrganizationID != orgID {
		return apperr.Forbidden("actor.wrong_tenant", "You do not have access to another organization's data.")
	}
	if action != "" && a.CanAny("pharmacy."+action, "vendor."+action) {
		return nil
	}
	return apperr.Forbidden("org.permission_required", "You do not have permission for this organization action.")
}
