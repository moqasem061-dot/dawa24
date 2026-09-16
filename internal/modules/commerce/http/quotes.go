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
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateQuoteRequest creates buyer price inquiry.
func (h *Handler) CreateQuoteRequest(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.Unauthorized())
		return
	}

	var q commerce.QuoteRequest
	if err := httpx.DecodeJSON(w, r, &q); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if actor.OrganizationID <= 0 {
		httpx.Error(w, r, h.log, apperr.Forbidden("quote.no_org", "Organization membership required to create quote requests"))
		return
	}
	q.CustomerOrgID = actor.OrganizationID

	created, err := h.service.CreateQuoteRequest(r.Context(), &q)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// RespondQuote allows vendor to respond with a quote.
func (h *Handler) RespondQuote(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid quote ID", nil))
		return
	}

	var body struct {
		Status        string       `json:"status"`
		QuotePrice    money.Amount `json:"quote_price"`
		SupplierNotes string       `json:"supplier_notes"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	// A supplier answers the quotes addressed to it, not another supplier's.
	actor, _ := authctx.From(r.Context())
	if !(actor.IsStaff && actor.CanAny("commerce.quote.manage", "commerce.admin")) {
		q, err := h.service.GetQuoteRequest(r.Context(), id)
		if err != nil || q == nil || actor.OrganizationID <= 0 || q.OrganizationID != actor.OrganizationID {
			httpx.Error(w, r, h.log, apperr.NotFound("quote_request"))
			return
		}
	}

	if err := h.service.RespondToQuote(r.Context(), id, commerce.QuoteStatus(body.Status), body.QuotePrice, body.SupplierNotes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListQuotes lists quotes for vendor or buyer.
func (h *Handler) ListQuotes(w http.ResponseWriter, r *http.Request) {
	// The organization is the caller's active tenant, resolved by middleware
	// after verifying membership. Reading it from ?org_id= let any caller name
	// any organization and read its quotes.
	orgID, ok := database.TenantFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, database.ErrNoTenant)
		return
	}

	isVendor := r.URL.Query().Get("role") == "vendor"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListQuoteRequests(r.Context(), orgID, isVendor, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"quotes": list, "count": len(list)})
}
