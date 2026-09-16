package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Review and finalisation.
//
// Nothing here touches commerce.carts. The review screen is its own thing: a
// smart order in progress is not the buyer's shopping cart, and mixing them
// would mean an abandoned import silently polluting a cart the buyer thought
// was empty. A test asserts no cart table is reachable from this path.

// Reviewer wires the finalisation use case into HTTP.
type Reviewer struct {
	svc       *smartorder.Service
	finalizer *smartorder.Finalizer
	handler   *Handler
}

// NewReviewer constructs the review handler.
func NewReviewer(h *Handler, svc *smartorder.Service, f *smartorder.Finalizer) *Reviewer {
	return &Reviewer{svc: svc, finalizer: f, handler: h}
}

func (v *Reviewer) lineID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "lineID"), 10, 64)
	if err != nil {
		v.handler.fail(w, r, apperr.NotFound("smart_order_line"))
		return 0, false
	}
	return id, true
}

// QuantityRequest is a review-time quantity edit.
type QuantityRequest struct {
	Quantity float64 `json:"quantity"`
}

// SetQuantity applies a quantity edit.
// POST /api/v1/smart-order/{id}/lines/{lineID}/quantity
func (v *Reviewer) SetQuantity(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := v.handler.actor(w, r)
	if !ok {
		return
	}
	run, ok := v.handler.run(w, r, orgID)
	if !ok {
		return
	}
	lineID, ok := v.lineID(w, r)
	if !ok {
		return
	}
	var req QuantityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v.handler.fail(w, r, apperr.Validation("smartorder.bad_request", "could not read the quantity", nil))
		return
	}
	if err := v.svc.SetQuantity(r.Context(), orgID, lineID, req.Quantity); err != nil {
		v.handler.fail(w, r, err)
		return
	}
	v.respondTotals(w, r, run)
}

// SupplierRequest chooses a different vendor for a line.
type SupplierRequest struct {
	CandidateID int64 `json:"candidate_id"`
}

// ChooseSupplier overrides the automatic selection.
// POST /api/v1/smart-order/{id}/lines/{lineID}/supplier
func (v *Reviewer) ChooseSupplier(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := v.handler.actor(w, r)
	if !ok {
		return
	}
	run, ok := v.handler.run(w, r, orgID)
	if !ok {
		return
	}
	lineID, ok := v.lineID(w, r)
	if !ok {
		return
	}
	var req SupplierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		v.handler.fail(w, r, apperr.Validation("smartorder.bad_request", "could not read the choice", nil))
		return
	}
	if err := v.svc.ChooseSupplier(r.Context(), orgID, lineID, req.CandidateID); err != nil {
		v.handler.fail(w, r, err)
		return
	}
	v.respondTotals(w, r, run)
}

// RemoveLine drops a line from the order.
// POST /api/v1/smart-order/{id}/lines/{lineID}/remove
func (v *Reviewer) RemoveLine(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := v.handler.actor(w, r)
	if !ok {
		return
	}
	run, ok := v.handler.run(w, r, orgID)
	if !ok {
		return
	}
	lineID, ok := v.lineID(w, r)
	if !ok {
		return
	}
	if err := v.svc.RemoveLine(r.Context(), orgID, lineID); err != nil {
		v.handler.fail(w, r, err)
		return
	}
	v.respondTotals(w, r, run)
}

// CorrectMatch links a line to the right catalogue product.
// POST /api/v1/smart-order/{id}/lines/{lineID}/match
func (v *Reviewer) CorrectMatch(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := v.handler.actor(w, r)
	if !ok {
		return
	}
	lineID, ok := v.lineID(w, r)
	if !ok {
		return
	}
	var req struct {
		ProductID int64 `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProductID <= 0 {
		v.handler.fail(w, r, apperr.Validation("smartorder.bad_request",
			"choose the catalogue product this line refers to", nil))
		return
	}
	line, err := v.svc.CorrectMatch(r.Context(), orgID, lineID, req.ProductID)
	if err != nil {
		v.handler.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, line)
}

// Finalize re-verifies and places the order.
// POST /api/v1/smart-order/{id}/finalize
//
// A stale line returns 409 with the lines named, not a partial order. See
// smartorder.Finalizer for why nothing is substituted or dropped.
func (v *Reviewer) Finalize(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := v.handler.actor(w, r)
	if !ok {
		return
	}
	run, ok := v.handler.run(w, r, orgID)
	if !ok {
		return
	}

	orderID, stale, err := v.finalizer.Finalize(r.Context(), run)
	if err != nil {
		v.handler.fail(w, r, err)
		return
	}
	if len(stale) > 0 {
		httpx.JSON(w, http.StatusConflict, map[string]any{
			"error":       "smartorder.lines_changed",
			"message":     "some lines changed since this order was generated",
			"stale_lines": stale,
		})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"order_id": orderID, "run": run})
}

// respondTotals recomputes and returns the figures the review screen shows, so
// an edit updates the totals in the same round trip (FR-044).
func (v *Reviewer) respondTotals(w http.ResponseWriter, r *http.Request, run *smartorder.Run) {
	cfg, err := v.svc.Config(r.Context(), run.ID)
	if err != nil {
		v.handler.fail(w, r, err)
		return
	}
	total, err := v.svc.Recalculate(r.Context(), run, cfg)
	if err != nil {
		v.handler.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"estimated_total": total,
		"budget_exceeded": run.BudgetExceeded,
		"budget_overage":  run.BudgetOverage,
		"stats":           run.Stats,
	})
}
