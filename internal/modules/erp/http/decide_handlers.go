package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// decideBody is the only input of a decision. Any other field is ignored, so a
// caller cannot choose the organization, the shipment or the actor.
type decideBody struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// decideSale confirms or rejects one of the supplier's sales orders (S4). The
// status is either "confirmed" or "rejected"; the service refuses everything else.
func (h *Handler) decideSale(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeError(w, r, erp.NewError("ERP_AUTH_MISSING", 401, nil))
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		return // readBody already wrote the envelope
	}
	var body decideBody
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, r, erp.NewError("ERP_BAD_JSON", 400, map[string]any{"position": 0}))
		return
	}
	res, err := h.svc.Decide(r.Context(), principal, chi.URLParam(r, "orderNumber"), body.Status, body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, http.StatusOK, map[string]any{
		"orderNumber": res.OrderNumber,
		"result":      res.Result,
		"orderStatus": res.OrderStatus,
	})
}
