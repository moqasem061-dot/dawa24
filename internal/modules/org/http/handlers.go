package http

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Handler exposes organization RESTful endpoints.
type Handler struct {
	service *org.Service
	log     *slog.Logger
}

// NewHandler creates a new organization HTTP handler.
func NewHandler(service *org.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterPreApprovalRoutes registers routes accessible before organization approval
// (organization onboarding registration and viewing own organization status).
func (h *Handler) RegisterPreApprovalRoutes(r chi.Router) {
	r.Post("/api/v1/org/organizations", h.RegisterOrg)
	r.Get("/api/v1/org/organizations/{id}", h.GetOrg)
}

// RegisterApprovedRoutes registers routes requiring an approved organization.
func (h *Handler) RegisterApprovedRoutes(r chi.Router) {
	r.Put("/api/v1/org/organizations/{id}", h.UpdateOrg)
	r.Delete("/api/v1/org/organizations/{id}", h.DeleteOrg)
	r.Get("/api/v1/org/organizations", h.ListOrgs)
	r.Post("/api/v1/org/organizations/{id}/status", h.UpdateStatus)

	r.Post("/api/v1/org/organizations/{id}/branches", h.CreateBranch)
	r.Get("/api/v1/org/organizations/{id}/branches", h.ListBranches)
	r.Put("/api/v1/org/organizations/{id}/branches/{bid}", h.UpdateBranch)
	r.Delete("/api/v1/org/organizations/{id}/branches/{bid}", h.DeleteBranch)

	r.Post("/api/v1/org/organizations/{id}/members", h.AddMember)
	r.Get("/api/v1/org/organizations/{id}/members", h.ListMembers)
	r.Put("/api/v1/org/organizations/{id}/members/{uid}", h.UpdateMemberRole)
	r.Delete("/api/v1/org/organizations/{id}/members/{uid}", h.RemoveMember)

	r.Post("/api/v1/org/organizations/{id}/reviews", h.AddReview)
	r.Get("/api/v1/org/organizations/{id}/reviews", h.ListReviews)
	r.Post("/api/v1/org/organizations/{id}/follow", h.ToggleFollow)

	h.RegisterAdminRoutes(r)
}

// RegisterRoutes registers organization routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	h.RegisterPreApprovalRoutes(r)
	h.RegisterApprovedRoutes(r)
}

// RegisterOrg handles organization creation.
func (h *Handler) RegisterOrg(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegalName          string       `json:"legal_name"`
		TradeName          i18n.Text    `json:"trade_name"`
		TaxNumber          string       `json:"tax_number"`
		CommercialRegister string       `json:"commercial_register"`
		Type               string       `json:"type"`
		CreditLimit        money.Amount `json:"credit_limit"`
		PaymentTermsDays   int          `json:"payment_terms_days"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	o, err := h.service.RegisterOrganization(r.Context(), org.RegisterOrgInput{
		LegalName:          body.LegalName,
		TradeName:          body.TradeName,
		TaxNumber:          body.TaxNumber,
		CommercialRegister: body.CommercialRegister,
		Type:               org.OrganizationType(body.Type),
		CreditLimit:        body.CreditLimit,
		PaymentTermsDays:   body.PaymentTermsDays,
	})
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, o)
}

// GetOrg retrieves an organization by ID.
func (h *Handler) GetOrg(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	// Its own members read the record (registration status is shown before
	// approval), and platform staff with the view key; nobody else.
	if a, ok := authctx.From(r.Context()); !ok || !((a.IsStaff && a.Can("org.organization.view")) || a.OrganizationID == id) {
		httpx.Error(w, r, h.log, apperr.NotFound("organization"))
		return
	}
	o, err := h.service.GetOrganization(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, o)
}

// ListOrgs returns filtered organizations.
func (h *Handler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	// Every organisation's record, pending ones included: platform staff only.
	if a, ok := authctx.From(r.Context()); !ok || !a.IsStaff || !a.Can("org.organization.view") {
		httpx.Error(w, r, h.log, apperr.Forbidden("org.permission_required", "Only platform staff can list organizations."))
		return
	}
	var orgType *org.OrganizationType
	if t := r.URL.Query().Get("type"); t != "" {
		ot := org.OrganizationType(t)
		orgType = &ot
	}

	var status *org.OrganizationStatus
	if s := r.URL.Query().Get("status"); s != "" {
		os := org.OrganizationStatus(s)
		status = &os
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListOrganizations(r.Context(), orgType, status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"organizations": list, "count": len(list)})
}

// UpdateStatus modifies organization status.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	// Status changes (approving, rejecting, suspending an organization) strictly
	// require platform staff with org.admin permission. A tenant member cannot self-approve.
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}
	if !actor.IsStaff || !actor.Can("org.admin") {
		httpx.Error(w, r, h.log, apperr.Forbidden("org.admin_required", "Only platform administrators can change organization approval status."))
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var opErr error
	switch org.OrganizationStatus(body.Status) {
	case org.StatusApproved:
		opErr = h.service.ApproveOrganization(r.Context(), id)
	case org.StatusRejected:
		opErr = h.service.RejectOrganization(r.Context(), id)
	case org.StatusSuspended:
		opErr = h.service.SuspendOrganization(r.Context(), id)
	default:
		opErr = apperr.Validation("status.invalid", "Invalid status value", nil)
	}

	if opErr != nil {
		httpx.Error(w, r, h.log, opErr)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// CreateBranch adds a branch.
func (h *Handler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	// Guarded for the same reason as the handlers in mutations.go: the id comes
	// from the URL, so without this any authenticated user could act on any
	// organization. Status changes belong to platform staff, who hold org.admin.
	if err := orgAccess(r.Context(), orgID, "branch.create", "org.branch.update"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var b org.Branch
	if err := httpx.DecodeJSON(w, r, &b); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	b.OrganizationID = orgID

	if err := h.service.CreateBranch(r.Context(), &b); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, b)
}

// ListBranches returns branches.
func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	if err := orgAccess(r.Context(), orgID, "branch.view", "org.branch.view"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	list, err := h.service.ListBranches(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"branches": list, "count": len(list)})
}

// AddMember adds a member.
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	// Guarded for the same reason as the handlers in mutations.go: the id comes
	// from the URL, so without this any authenticated user could act on any
	// organization. Status changes belong to platform staff, who hold org.admin.
	if err := orgAccess(r.Context(), orgID, "team.create", "org.member.manage"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		UserID int64 `json:"user_id"`
		RoleID int64 `json:"role_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	m, err := h.service.AddMember(r.Context(), orgID, body.UserID, body.RoleID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, m)
}

// ListMembers lists members.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	if err := orgAccess(r.Context(), orgID, "team.view", "org.organization.view"); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	list, err := h.service.ListMembers(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"members": list, "count": len(list)})
}

// AddReview records a review.
func (h *Handler) AddReview(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	var body struct {
		UserID int64  `json:"user_id"`
		Rating int    `json:"rating"`
		Text   string `json:"review_text"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	rev, err := h.service.AddReview(r.Context(), orgID, body.UserID, body.Rating, body.Text)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, rev)
}

// ListReviews lists reviews.
func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid org ID", nil))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListReviews(r.Context(), orgID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"reviews": list, "count": len(list)})
}

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
