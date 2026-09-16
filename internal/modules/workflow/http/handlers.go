package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes workflow HTTP endpoints.
type Handler struct {
	service *workflow.Service
	log     *slog.Logger
}

// NewHandler creates a workflow HTTP handler.
func NewHandler(service *workflow.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers workflow routes on a Chi router.
//
// Weekly coverage decides which pharmacies a supplier delivers to and therefore
// what appears in their catalogue. It was writable over JSON by any approved
// organisation member, taking the branch from the URL, while /vendor/coverage
// requires vendor.coverage.manage.
//
// Reporting an issue stays open to any authenticated member: refusing a
// complaint on a permission is how a platform stops hearing about its faults.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/workflow/branches/{id}/coverage", h.GetBranchCoverage)
	r.Get("/api/v1/workflow/issues", h.ListIssues)
	r.Post("/api/v1/workflow/issues", h.ReportIssue)

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission(
			"vendor.coverage.manage", "workflow.coverage.update", "workflow.admin"))
		g.Post("/api/v1/workflow/branches/{id}/coverage", h.SetWeeklyCoverage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission(
			"pharmacy.purchase_request.create", "vendor.purchase_request.respond",
			"workflow.request.update", "workflow.admin"))
		g.Post("/api/v1/workflow/priority-requests", h.CreatePriorityRequest)
	})

	h.RegisterAdminRoutes(r)
}

// CreatePriorityRequest submits a purchasing priority calculation.
func (h *Handler) CreatePriorityRequest(w http.ResponseWriter, r *http.Request) {
	var req workflow.PurchasePriorityRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreatePriorityRequest(r.Context(), req.UserID, &req)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// SetWeeklyCoverage configures branch weekly route coverage.
func (h *Handler) SetWeeklyCoverage(w http.ResponseWriter, r *http.Request) {
	branchIDStr := chi.URLParam(r, "id")
	branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("branch_id.invalid", "Invalid branch ID", nil))
		return
	}

	var c workflow.WeeklyCoverage
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	c.BranchID = branchID

	if err := h.service.SetWeeklyCoverage(r.Context(), &c); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetBranchCoverage lists branch weekly route coverage.
func (h *Handler) GetBranchCoverage(w http.ResponseWriter, r *http.Request) {
	branchIDStr := chi.URLParam(r, "id")
	branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("branch_id.invalid", "Invalid branch ID", nil))
		return
	}

	coverage, err := h.service.GetBranchCoverage(r.Context(), branchID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"coverage": coverage})
}

// ReportIssue logs a support issue.
func (h *Handler) ReportIssue(w http.ResponseWriter, r *http.Request) {
	var i workflow.ReportIssue
	if err := httpx.DecodeJSON(w, r, &i); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.ReportIssue(r.Context(), &i)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// ListIssues returns issues.
func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	issues, err := h.service.ListIssues(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"issues": issues})
}
