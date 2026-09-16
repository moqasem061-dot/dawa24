package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// FavoritesPage lists the signed-in user's favourited products.
func (h *UIHandler) FavoritesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	var products []*catalog.Product
	if h.idSvc != nil && h.catSvc != nil {
		ids, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		if err != nil {
			h.log.WarnContext(ctx, "favorites: list favorites", "error", err)
		} else if len(ids) > 0 {
			// One query for the whole list. This was a GetProduct per
			// favourite, against a fifty-thousand-row table, on a page a
			// pharmacy opens to see everything it saved.
			byID, prodErr := h.catSvc.ProductsByIDs(ctx, ids)
			if prodErr != nil {
				h.log.WarnContext(ctx, "favorites: resolve products", "error", prodErr)
			}
			// Ranged over ids, not the map, so the page keeps the order the
			// favourites came back in.
			for _, id := range ids {
				if p := byID[id]; p != nil {
					products = append(products, p)
				}
			}
		}
	}

	h.renderPage(ctx, w, "render favorites page", pages.FavoritesPage(lang, dir, products))
}

// FavoriteRemoveSubmit removes a product from the user's favourites.
func (h *UIHandler) FavoriteRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			h.log.DebugContext(ctx, "favorite remove: invalid id", "error", err)
		} else {
			if err := h.idSvc.RemoveFavorite(ctx, actor.UserID, id); err != nil {
				h.log.WarnContext(ctx, "favorite remove: failed", "id", id, "error", err)
			}
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteAddSubmit adds a product to the user's favourites.
func (h *UIHandler) FavoriteAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	if h.idSvc != nil && productID > 0 {
		if err := h.idSvc.AddFavorite(ctx, actor.UserID, productID); err != nil {
			h.log.WarnContext(ctx, "favorite add: failed", "product_id", productID, "error", err)
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteToggleSubmit toggles a product in the user's favourites.
func (h *UIHandler) FavoriteToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/favorites")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	isFav := false
	if h.idSvc != nil && productID > 0 {
		favs, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		if err != nil {
			h.log.WarnContext(ctx, "favorite toggle: list favorites", "error", err)
		} else {
			for _, id := range favs {
				if id == productID {
					isFav = true
					break
				}
			}
		}
		if isFav {
			_ = h.idSvc.RemoveFavorite(ctx, actor.UserID, productID)
			isFav = false
		} else {
			_ = h.idSvc.AddFavorite(ctx, actor.UserID, productID)
			isFav = true
		}
	}

	isJSON := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"
	if h.isHTMX(r) || isJSON {
		msg := i18n.T(langOf(r), "favorites.added")
		if !isFav {
			msg = i18n.T(langOf(r), "favorites.removed")
		}
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"info"},"favoriteToggled":{"productId":%d,"isFavorite":%t}}`, msg, productID, isFav))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "is_favorite": isFav, "product_id": productID, "message": msg})
		return
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
