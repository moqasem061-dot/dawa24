package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// RegisterAdminRoutes mounts administrative inventory routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("inventory.admin"))

		admin.Get("/api/v1/admin/inventory/warehouses", h.AdminListWarehouses)
		admin.Get("/api/v1/admin/inventory/transfers", h.AdminListTransfers)
	})
}

func (h *Handler) AdminListWarehouses(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	warehouses, err := h.service.ListWarehouses(ctx)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"warehouses": warehouses})
}

func (h *Handler) AdminListTransfers(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	transfers, err := h.service.ListTransfers(ctx, status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"transfers": transfers})
}
