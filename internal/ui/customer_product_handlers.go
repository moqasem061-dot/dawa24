package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// /catalog/{id} is authenticated-only, matching the listing page.
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.catSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	// Try lookup as variant first, then fallback to product
	var targetProductID int64
	var targetVariantID int64

	if variant, err := h.catSvc.GetVariant(ctx, id); err == nil && variant != nil {
		targetProductID = variant.ProductID
		targetVariantID = variant.ID
	} else {
		targetProductID = id
	}

	if actor.IsStaff {
		http.Redirect(w, r, fmt.Sprintf("/admin/products/%d", targetProductID), http.StatusSeeOther)
		return
	}

	product, variants, err := h.catSvc.GetProduct(ctx, targetProductID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	env := h.buildOfferEnv(ctx,
		[]int64{product.ID},
		map[int64][]*catalog.ProductVariant{product.ID: variants},
	)
	offers := h.offersForProduct(ctx, product, variants, env, lang)

	if targetVariantID > 0 {
		for i := range offers {
			if offers[i].VariantID == targetVariantID {
				if i > 0 {
					focused := offers[i]
					copy(offers[1:i+1], offers[0:i])
					offers[0] = focused
				}
				break
			}
		}
	} else {
		// When browsing general product, prioritize covered offers at the top
		sort.SliceStable(offers, func(i, j int) bool {
			if offers[i].IsCovered != offers[j].IsCovered {
				return offers[i].IsCovered
			}
			return false
		})
	}

	h.renderPage(ctx, w, "render product detail page", pages.CustomerProductDetail(product, variants, offers, lang, dir))
}
