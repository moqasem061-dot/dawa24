package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
)

// Warehouse lifecycle, replenishment and transfer-lifecycle endpoints.
// Split from handlers.go to stay within the 400-line file limit.

// pathID parses a numeric path parameter, returning a validation error rather
// than a 500 when a client sends something that is not a number.
func pathID(r *http.Request, name, entity string) (int64, error) {
	raw := chi.URLParam(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation("id.invalid", "Invalid "+entity+" identifier.",
			map[string]string{name: "must be a positive integer"})
	}
	return id, nil
}

// GetWarehouse returns one warehouse.
func (h *Handler) GetWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "warehouse")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	warehouse, err := h.service.GetWarehouse(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, warehouse)
}

// UpdateWarehouse edits an existing warehouse.
func (h *Handler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "warehouse")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var input inventory.Warehouse
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	updated, err := h.service.UpdateWarehouse(r.Context(), id, &input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// DeleteWarehouse soft-deletes an empty warehouse.
func (h *Handler) DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "warehouse")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.DeleteWarehouse(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListLowStock returns stock at or below its reorder threshold.
func (h *Handler) ListLowStock(w http.ResponseWriter, r *http.Request) {
	p := pagination.FromRequest(r)

	stocks, err := h.service.ListLowStock(r.Context(), p.Limit, p.Offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, pagination.Page[*inventory.Stock]{
		Data:    stocks,
		HasMore: len(stocks) == p.Limit,
	})
}

// ListOrgMovements returns the organisation-wide stock ledger.
func (h *Handler) ListOrgMovements(w http.ResponseWriter, r *http.Request) {
	p := pagination.FromRequest(r)

	movements, err := h.service.ListOrgMovements(r.Context(), p.Limit, p.Offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, pagination.Page[*inventory.StockMovement]{
		Data:    movements,
		HasMore: len(movements) == p.Limit,
	})
}

// ListTransfers returns transfers, optionally filtered by ?status=.
func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	p := pagination.FromRequest(r)
	status := r.URL.Query().Get("status")

	transfers, err := h.service.ListTransfers(r.Context(), status, p.Limit, p.Offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, pagination.Page[*inventory.WarehouseTransfer]{
		Data:    transfers,
		HasMore: len(transfers) == p.Limit,
	})
}

// GetTransfer returns one transfer.
func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "transfer")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	transfer, err := h.service.GetTransfer(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, transfer)
}

// ReceiveTransfer credits the destination warehouse, completing the move.
func (h *Handler) ReceiveTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "transfer")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	transfer, err := h.service.ReceiveTransfer(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, transfer)
}

// CancelTransfer aborts a transfer and returns stock to the source.
func (h *Handler) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id", "transfer")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// A cancellation reason is optional, and an empty body is a legitimate
	// request, so a decode failure is tolerated here rather than rejected.
	_ = httpx.DecodeJSON(w, r, &body)

	transfer, err := h.service.CancelTransfer(r.Context(), id, body.Reason)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, transfer)
}
