package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// Searching a supplier's own stock, on demand.
//
// The advertisement wizard used to serialise every in-stock variant the company
// owns into an x-data attribute — InStockItemsToJSON(data.ItemOptions) — and
// filter that array in the browser. For a supplier with nine thousand items
// that is the whole inventory inlined into the HTML of a page whose purpose is
// to pick one of them, downloaded on every render whether or not the wizard is
// ever opened.
//
// This answers one query at a time, in the shape components.Combobox consumes.

// VendorStockSearchJSON returns the caller's own variants matching a query.
func (h *UIHandler) VendorStockSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode([]components.ComboboxOption{})
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 || h.catSvc == nil {
		_ = json.NewEncoder(w).Encode([]components.ComboboxOption{})
		return
	}
	// A search box is not a bulk export: a long query is truncated rather than
	// handed to the matcher whole.
	if len(query) > 80 {
		query = query[:80]
	}

	// in_stock=1 is what the advertisement wizard asks for: an advertisement is
	// only allowed against something the supplier can actually ship.
	onlyInStock := r.URL.Query().Get("in_stock") == "1"

	variants, _, err := h.catSvc.ListVendorVariants(ctx, actor.OrganizationID, catalog.VendorVariantQuery{
		Query:      query,
		PageNumber: 1,
		PerPage:    25,
		Stock: func() catalog.StockFilter {
			if onlyInStock {
				return catalog.StockFilterIn
			}
			return catalog.StockFilterAny
		}(),
	})
	if err != nil {
		h.log.ErrorContext(ctx, "vendor stock search", "error", err,
			"organization_id", actor.OrganizationID)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode([]components.ComboboxOption{})
		return
	}

	options := make([]components.ComboboxOption, 0, len(variants))
	for _, v := range variants {
		if v == nil {
			continue
		}
		if onlyInStock && v.StockQty <= 0 {
			continue
		}
		label := v.Name.Get(i18n.AR)
		if label == "" {
			label = v.Name.Get(i18n.EN)
		}
		if label == "" {
			label = v.SKU
		}

		hint := v.SKU
		if v.Unit != "" {
			hint = strings.TrimSpace(hint + " · " + v.Unit)
		}

		options = append(options, components.ComboboxOption{
			ID:    fmt.Sprintf("%d", v.ID),
			Label: label,
			Hint:  hint,
			Badge: fmt.Sprintf("%d متاح · %s ج.م", v.StockQty, v.Price.String()),
		})
	}

	_ = json.NewEncoder(w).Encode(options)
}
