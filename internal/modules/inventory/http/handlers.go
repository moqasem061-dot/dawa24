package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes inventory management HTTP endpoints.
type Handler struct {
	service *inventory.Service
	log     *slog.Logger
}

// NewHandler creates an inventory HTTP handler.
func NewHandler(service *inventory.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers inventory endpoints on a Chi router.
//
// Every write here was mounted behind authentication and an approved-org check
// and nothing more, so any employee of any approved organisation could create a
// warehouse, adjust a stock balance or cancel somebody's transfer over JSON —
// while the same actions on the HTML screens have always required
// vendor.warehouse.manage or vendor.inventory.adjust.
//
// Row-level security still scopes what a query can touch to the caller's own
// tenant, so this was not a cross-tenant hole; it was a within-tenant one, and
// that is the one that matters here. A pharmacy's delivery driver holds an
// approved-organisation session. Stock adjustment is how inventory is written
// off.
//
// RequirePermission takes both audiences: a staff member holding
// inventory.warehouse.manage administers a tenant's warehouses from /admin, and
// a vendor's own clerk holds vendor.warehouse.manage.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// --- reads -------------------------------------------------------------
	r.Get("/api/v1/inventory/warehouses", h.ListWarehouses)
	r.Get("/api/v1/inventory/warehouses/{id}", h.GetWarehouse)
	r.Get("/api/v1/inventory/warehouses/{id}/stocks", h.ListStocks)
	r.Get("/api/v1/inventory/stocks/low", h.ListLowStock)
	r.Get("/api/v1/inventory/stocks/{id}/movements", h.ListMovements)
	r.Get("/api/v1/inventory/movements", h.ListOrgMovements)
	r.Get("/api/v1/inventory/transfers", h.ListTransfers)
	r.Get("/api/v1/inventory/transfers/{id}", h.GetTransfer)

	// --- warehouses --------------------------------------------------------
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.warehouse.manage", "inventory.warehouse.manage", "inventory.admin"))
		g.Post("/api/v1/inventory/warehouses", h.CreateWarehouse)
		g.Put("/api/v1/inventory/warehouses/{id}", h.UpdateWarehouse)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.warehouse.manage", "inventory.warehouse.delete", "inventory.admin"))
		g.Delete("/api/v1/inventory/warehouses/{id}", h.DeleteWarehouse)
	})

	// --- balances ----------------------------------------------------------
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.inventory.adjust", "inventory.stock.adjust", "inventory.admin"))
		g.Post("/api/v1/inventory/stocks/adjust", h.AdjustStock)
	})

	// --- transfers ---------------------------------------------------------
	//
	// Two-phase: POST dispatches (source deducted), receive credits the
	// destination. See inventory/transfer_state.go. Receiving and cancelling
	// move stock as surely as dispatching does, so all three take the same
	// permission rather than only the first.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.inventory.adjust", "inventory.transfer.create", "inventory.admin"))
		g.Post("/api/v1/inventory/transfers", h.TransferStock)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.inventory.adjust", "inventory.transfer.approve",
			"inventory.transfer.update", "inventory.admin"))
		g.Post("/api/v1/inventory/transfers/{id}/receive", h.ReceiveTransfer)
		g.Post("/api/v1/inventory/transfers/{id}/cancel", h.CancelTransfer)
	})

	h.RegisterAdminRoutes(r)
}

// CreateWarehouse handles warehouse creation.
func (h *Handler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	var warehouse inventory.Warehouse
	if err := httpx.DecodeJSON(w, r, &warehouse); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateWarehouse(r.Context(), &warehouse)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// ListWarehouses lists tenant warehouses.
func (h *Handler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	warehouses, err := h.service.ListWarehouses(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"warehouses": warehouses,
	})
}

// ListStocks lists stocks in a warehouse.
func (h *Handler) ListStocks(w http.ResponseWriter, r *http.Request) {
	warehouseIDStr := chi.URLParam(r, "id")
	warehouseID, err := strconv.ParseInt(warehouseIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid warehouse ID", nil))
		return
	}

	// The warehouse must be the caller's: the listing query itself is not
	// scoped, and row-level security is not enforced for this connection.
	if wh, err := h.service.GetWarehouse(r.Context(), warehouseID); err != nil || wh == nil || !ownsInventory(r, wh.OrganizationID) {
		httpx.Error(w, r, h.log, apperr.NotFound("warehouse"))
		return
	}
	stocks, err := h.service.ListStocksByWarehouse(r.Context(), warehouseID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"stocks": stocks,
	})
}

// AdjustStock applies a manual or automated delta to a stock level.
func (h *Handler) AdjustStock(w http.ResponseWriter, r *http.Request) {
	var input inventory.AdjustStockInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	stock, err := h.service.AdjustStock(r.Context(), input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, stock)
}

// TransferStock executes an inter-warehouse stock transfer.
func (h *Handler) TransferStock(w http.ResponseWriter, r *http.Request) {
	var transfer inventory.WarehouseTransfer
	if err := httpx.DecodeJSON(w, r, &transfer); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	result, err := h.service.TransferStock(r.Context(), &transfer)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, result)
}

// ListMovements returns the movement ledger history for a stock item.
func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	stockIDStr := chi.URLParam(r, "id")
	stockID, err := strconv.ParseInt(stockIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid stock ID", nil))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if st, err := h.service.GetStockByID(r.Context(), stockID); err != nil || st == nil || !ownsInventory(r, st.OrganizationID) {
		httpx.Error(w, r, h.log, apperr.NotFound("stock"))
		return
	}
	movements, err := h.service.ListStockMovements(r.Context(), stockID, limit)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"movements": movements,
	})
}

// ownsInventory reports whether a record of this organisation belongs to the
// caller, or the caller is platform staff with inventory administration.
func ownsInventory(r *http.Request, orgID int64) bool {
	actor, ok := authctx.From(r.Context())
	if !ok {
		return false
	}
	if actor.IsStaff && actor.Can("inventory.admin") {
		return true
	}
	return actor.OrganizationID > 0 && actor.OrganizationID == orgID
}
