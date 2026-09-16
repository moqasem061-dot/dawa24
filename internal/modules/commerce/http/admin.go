package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequireAPIPermission("commerce.admin"))

		admin.Get("/api/v1/admin/commerce/orders", h.AdminSearchOrders)
		admin.Get("/api/v1/admin/commerce/orders/{id}", h.AdminGetOrder)
		admin.Post("/api/v1/admin/commerce/orders/{id}/status", h.AdminForceOrderStatus)
		admin.Post("/api/v1/admin/commerce/orders/{id}/refund", h.AdminRefundOrder)
	})
}

func (h *Handler) AdminSearchOrders(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())

	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	orders, err := h.service.AdminSearchOrders(ctx, query, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (h *Handler) AdminGetOrder(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	order, err := h.service.GetOrder(ctx, id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, order)
}

func (h *Handler) AdminForceOrderStatus(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.TransitionOrderStatus(ctx, id, commerce.OrderStatus(body.Status), &actorID, body.Note); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) AdminRefundOrder(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = httpx.DecodeJSON(w, r, &body)

	if err := h.service.TransitionOrderStatus(ctx, id, commerce.StatusRefunded, &actorID, "Refund: "+body.Reason); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "refunded"})
}
