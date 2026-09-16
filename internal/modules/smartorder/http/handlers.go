// Package http exposes the smart ordering wizard on the customer surface.
//
// Every handler resolves the run by public id **and** the caller's organisation.
// A run belonging to someone else is Not Found, never Forbidden: a 403 confirms
// the run exists, which is a disclosure the buyer is not entitled to.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Handler serves the smart ordering API.
type Handler struct {
	svc *smartorder.Service
	log *slog.Logger
}

// NewHandler constructs the handler.
func NewHandler(svc *smartorder.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// fail renders an error through the platform's classifier, so an apperr becomes
// the right status and a bare error never leaks its text to a buyer.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.Error(w, r, h.log, err)
}

// actor pulls the caller's identity, or writes 401 and reports false.
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	a, ok := authctx.From(r.Context())
	if !ok {
		h.fail(w, r, apperr.Forbidden("auth.required", "sign in to continue"))
		return 0, 0, false
	}
	orgID := a.OrgID
	if orgID <= 0 {
		orgID = a.OrganizationID
	}
	if orgID <= 0 {
		h.fail(w, r, apperr.Forbidden("smartorder.no_organization",
			"smart ordering is available to organisation members"))
		return 0, 0, false
	}
	return orgID, a.UserID, true
}

// run resolves the run in the URL, scoped to the caller.
func (h *Handler) run(w http.ResponseWriter, r *http.Request, orgID int64) (*smartorder.Run, bool) {
	run, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return nil, false
	}
	return run, true
}

// StartRequest is the import screen's payload.
type StartRequest struct {
	BranchID          int64    `json:"branch_id"`
	UploadID          *int64   `json:"upload_id"`
	Filename          string   `json:"filename"`
	Criteria          []string `json:"criteria"`
	TolerancePct      float64  `json:"tolerance_pct"`
	DefaultQuantity   int      `json:"default_quantity"`
	MaxBudget         string   `json:"max_budget"`
	UseSavingProducts bool     `json:"use_saving_products"`
	UseAIMatching     bool     `json:"use_ai_matching"`
	MinMatchScore     float64  `json:"min_match_score"`
}

// Start creates a run. POST /api/v1/smart-order
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.fail(w, r, apperr.Validation("smartorder.bad_request",
			"could not read the request", nil))
		return
	}

	var budget *money.Amount
	if s := strings.TrimSpace(req.MaxBudget); s != "" {
		amt, err := money.Parse(s)
		if err != nil {
			h.fail(w, r, apperr.Validation("smartorder.bad_budget",
				"the maximum budget is not a valid amount", nil))
			return
		}
		budget = &amt
	}

	if req.DefaultQuantity <= 0 {
		req.DefaultQuantity = 1
	}

	criteria := make([]smartorder.Criterion, 0, len(req.Criteria))
	for _, c := range req.Criteria {
		criteria = append(criteria, smartorder.Criterion(c))
	}

	run, err := h.svc.Start(r.Context(), smartorder.StartOptions{
		UserID:            userID,
		OrganizationID:    orgID,
		BranchID:          req.BranchID,
		UploadID:          req.UploadID,
		Filename:          req.Filename,
		Criteria:          criteria,
		TolerancePct:      req.TolerancePct,
		DefaultQuantity:   req.DefaultQuantity,
		MaxBudget:         budget,
		UseSavingProducts: req.UseSavingProducts,
		UseAIMatching:     req.UseAIMatching,
		MinMatchScore:     req.MinMatchScore,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, run)
}

// Get returns a run. GET /api/v1/smart-order/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, ok := h.run(w, r, orgID)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, run)
}

// History lists previous runs. GET /api/v1/smart-order
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	runs, err := h.svc.History(r.Context(), orgID, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// MappingRequest confirms the column mapping.
type MappingRequest struct {
	HeaderRow int               `json:"header_row"`
	Mapping   map[string]string `json:"mapping"`
}

// ConfirmMapping saves the mapping and readies the run.
// POST /api/v1/smart-order/{id}/mapping
func (h *Handler) ConfirmMapping(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, ok := h.run(w, r, orgID)
	if !ok {
		return
	}
	var req MappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.fail(w, r, apperr.Validation("smartorder.bad_request",
			"could not read the mapping", nil))
		return
	}

	fields := make(map[int]string, len(req.Mapping))
	for col, field := range req.Mapping {
		n, err := strconv.Atoi(col)
		if err != nil {
			continue
		}
		if strings.TrimSpace(field) != "" {
			fields[n] = field
		}
	}

	m := &smartorder.Mapping{
		HeaderRow:      req.HeaderRow,
		Fields:         fields,
		UserOverridden: true,
	}
	if err := h.svc.ConfirmMapping(r.Context(), run, m); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Queue(r.Context(), run); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, run)
}

// Results returns a filtered page of lines.
// GET /api/v1/smart-order/{id}/results
func (h *Handler) Results(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, ok := h.run(w, r, orgID)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("per_page"))
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}

	lines, total, err := h.svc.Results(r.Context(), run, smartorder.LineFilter{
		MatchGroup: q.Get("match"),
		Outcome:    q.Get("outcome"),
		Method:     q.Get("method"),
		Search:     q.Get("q"),
		Limit:      limit,
		Offset:     (page - 1) * limit,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"lines": lines, "total": total, "page": page, "per_page": limit,
		"stats": run.Stats, "estimated_total": run.EstimatedTotal,
		"budget_exceeded": run.BudgetExceeded, "budget_overage": run.BudgetOverage,
	})
}

// Candidates lists every vendor considered for a line, rejected ones included.
// GET /api/v1/smart-order/{id}/lines/{lineID}/candidates
func (h *Handler) Candidates(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	lineID, err := strconv.ParseInt(chi.URLParam(r, "lineID"), 10, 64)
	if err != nil {
		h.fail(w, r, apperr.NotFound("smart_order_line"))
		return
	}
	candidates, err := h.svc.Candidates(r.Context(), orgID, lineID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	smartorder.SortCandidatesForDisplay(candidates)
	httpx.JSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}
