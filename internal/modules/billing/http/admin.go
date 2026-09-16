package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("billing.admin"))

		admin.Get("/api/v1/admin/billing/subscriptions", h.AdminListSubscriptions)
		admin.Post("/api/v1/admin/billing/wallets/{id}/adjust", h.AdminAdjustWallet)
		admin.Get("/api/v1/admin/billing/payments", h.AdminListPayments)
		admin.Post("/api/v1/admin/billing/invoices/{id}/paid", h.AdminMarkInvoicePaid)
	})
}

func (h *Handler) AdminListSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	subscriptions, err := h.service.AdminListSubscriptions(ctx, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"subscriptions": subscriptions})
}

func (h *Handler) AdminAdjustWallet(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid wallet ID", nil))
		return
	}

	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Amount money.Amount `json:"amount"`
		Reason string       `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.AdminAdjustWallet(ctx, id, body.Amount, body.Reason, actorID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "adjusted"})
}

func (h *Handler) AdminListPayments(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	payments, err := h.service.AdminListPayments(ctx, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"payments": payments})
}

func (h *Handler) AdminMarkInvoicePaid(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid invoice ID", nil))
		return
	}

	if err := h.service.MarkInvoicePaid(ctx, id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "paid"})
}
