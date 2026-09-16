package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The shared buying surface.
//
// These routes used to live in registerCustomerRoutes, behind RequireCustomer,
// which answered a supplier 404 for every one of them. That was right while
// only pharmacies bought; it stopped being right when suppliers began
// restocking from other distributors, and copying the pages into /vendor/*
// would have produced a second catalogue, a second cart and a second place for
// the self-supply rule to be forgotten.
//
// So there is one set of pages, mounted once, gated by RequireBuyer. Each group
// names a rbac.Capability rather than a permission key, and the gate resolves
// the key the caller's own dashboard grants it under — pharmacy.order.view for
// a pharmacist, vendor.buying.order.view for a supplier's buyer.
//
// The /customer/ prefix on some paths is a URL, not an audience: those paths
// predate this split and are kept because bookmarks, templates and the Smart
// Ordering wizard's own links point at them. What decides who may open them is
// the gate below, and nothing else.
func (h *UIHandler) registerBuyingRoutes(r chi.Router) {
	h.registerBuyingCatalogRoutes(r)
	h.registerBuyingCartRoutes(r)
	h.registerBuyingOrderRoutes(r)
	h.registerBuyingMarketRoutes(r)
	h.registerBuyingBranchRoutes(r)
}

// registerBuyingCatalogRoutes is the catalogue and the purchase-request
// gateway: browsing what the market sells, and deciding what to ask for.
func (h *UIHandler) registerBuyingCatalogRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(h.scrape.Protect)
		g.Use(authctx.RequireCapability(rbac.BuyCatalogView))
		g.Get("/customer/catalog", h.CustomerCatalogPage)
	})

	// One product, and the two aliases that reach one, belong with the
	// catalogue that lists them.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyCatalogView))
		g.Get("/customer/catalog/{id}", h.CustomerProductDetailPage)
		g.Get("/customer/add-order", h.CustomerAddOrderPage)
		g.Get("/customer/products/main/{id}", h.CustomerProductsMainAlias)
	})

	// The purchase-request gateway. Its own grant, and Smart Ordering's,
	// because the page is where both start — the same two the sidebar item
	// names, so the link and the page agree.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyPurchaseRequestView, rbac.BuySmartOrderView))
		g.Get("/customer/purchase-request", h.CustomerPurchaseRequestWizardPage)
		g.Get("/customer/purchase-request/products", h.CustomerPurchaseRequestProductsRedirect)
		g.Get("/customer/purchase-request/previous", h.CustomerPurchaseRequestPreviousRedirect)
		g.Get("/customer/purchase-request/supplier", h.CustomerPurchaseRequestSupplierRedirect)
		g.Get("/customer/purchase-request/supplier/{id}", h.CustomerPurchaseRequestSupplierRedirect)
	})

	// The former Automatic Purchase Request feature is superseded by Smart
	// Ordering (specs/001-smart-ordering-system).
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuySmartOrderView))
		g.Get("/customer/automation", redirectTo("/customer/smart-order"))
		g.Get("/customer/automation/previous", redirectTo("/customer/smart-order/history"))
	})
}

// registerBuyingCartRoutes is the basket and the checkout.
//
// Filling a basket and committing the company's money are two grants, not one:
// a counter assistant may prepare an order for a pharmacist to approve without
// being able to submit it themselves.
func (h *UIHandler) registerBuyingCartRoutes(r chi.Router) {
	// Cart count badge is a header partial loaded in the top bar.
	// It handles unauthenticated or unauthorized callers gracefully with 200 OK
	// instead of kicking them into an unauthorized redirect loop.
	r.Get("/cart/count-badge", h.CartCountBadge)

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyCartUse))
		g.Get("/cart", h.CustomerCartPage)
		g.Post("/cart/add", h.AddToCartSubmit)
		g.Post("/cart/add-offer", h.AddOfferToCartSubmit)
		g.Post("/cart/remove", h.RemoveFromCartSubmit)
		g.Post("/cart/update-quantity", h.UpdateCartQuantitySubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyOrderCreate))
		g.Get("/checkout", h.CustomerCheckoutPage)
		g.Get("/offers/{id}/checkout", h.CustomerOfferCheckoutPage)
		g.Post("/checkout", h.CheckoutSubmit)
	})
}

// registerBuyingOrderRoutes is the buyer's own orders and shipments — what this
// company bought, not what it sold.
func (h *UIHandler) registerBuyingOrderRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if actor, ok := authctx.From(r.Context()); ok && (actor.IsStaff || actor.Can("commerce.order.view")) {
					if id := chi.URLParam(r, "id"); id != "" {
						http.Redirect(w, r, "/admin/orders/"+id, http.StatusTemporaryRedirect)
						return
					}
					http.Redirect(w, r, "/admin/orders", http.StatusTemporaryRedirect)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		g.Use(authctx.RequireCapability(rbac.BuyOrderView))
		g.Get("/orders", h.CustomerOrdersPage)
		g.Get("/orders/{id}", h.CustomerOrderDetailPage)
		g.Get("/orders/{id}/lines/{lineID}/offer-details", h.CustomerOrderLineOfferDetails)
		g.Get("/customer/orders", h.CustomerOrdersPage)
		g.Get("/customer/orders/{id}", h.CustomerOrderDetailPage)
		g.Get("/customer/orders/{id}/lines/{lineID}/offer-details", h.CustomerOrderLineOfferDetails)
		g.Get("/orders/offers", redirectTo("/orders"))
		g.Get("/orders/offers/{id}", redirectToOrder)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyOrderUpdate))
		g.Post("/orders/{id}/edit", h.CustomerOrderEditSubmit)
		g.Post("/customer/orders/{id}/edit", h.CustomerOrderEditSubmit)
		g.Post("/orders/{id}/cancel", h.CustomerOrderCancelSubmit)
		g.Post("/customer/orders/{id}/cancel", h.CustomerOrderCancelSubmit)
		g.Post("/customer/negotiate-order", h.CustomerNegotiateOrderSubmit)
	})
}

// registerBuyingMarketRoutes is the market a buyer shops in: the supplier
// directory, the offers board, the favourites list and the reviews a buyer
// leaves behind.
func (h *UIHandler) registerBuyingMarketRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(h.scrape.Protect)
		g.Use(authctx.RequireCapability(rbac.BuySupplierView))
		g.Get("/customer/suppliers", h.SuppliersPage)
		g.Get("/customer/suppliers/{id}", h.SupplierProfilePage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuySupplierFollow))
		g.Get("/suppliers/followed", h.FollowedSuppliersPage)
		g.Post("/suppliers/{id}/follow", h.SupplierFollowSubmit)
		g.Post("/suppliers/{id}/message", h.SupplierMessageSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyOfferView))
		g.Get("/customer/offers", h.OffersPage)
		g.Get("/customer/offers/{id}", h.OfferDetailPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyReviewWrite))
		g.Post("/reviews/submit", h.ReviewSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireCapability(rbac.BuyFavoriteView))
		g.Get("/favorites", h.FavoritesPage)
		g.Get("/customer/favorites", h.FavoritesPage)
		g.Post("/favorites/{id}/remove", h.FavoriteRemoveSubmit)
		g.Post("/favorites/{id}/add", h.FavoriteAddSubmit)
		g.Post("/favorites/{id}/toggle", h.FavoriteToggleSubmit)
		g.Post("/favorites/toggle", h.FavoriteToggleSubmit)
	})
}

// registerBuyingBranchRoutes is the receiving-branch choice.
//
// Choosing which of your own branches an order is delivered to is not a
// privilege: every member who can order needs it, and it addresses only
// branches the company already owns — the handlers re-read the branch and
// refuse one belonging to anybody else.
func (h *UIHandler) registerBuyingBranchRoutes(r chi.Router) {
	r.Post("/customer/set-branch", h.SetBuyingBranchSubmit)
	r.Post("/customer/branches/active", h.CustomerSwitchActiveBranchSubmit)
}

// redirectTo builds a permanent redirect handler for a path that moved.
func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

// redirectToOrder sends a legacy /orders/offers/{id} link to the order itself.
func redirectToOrder(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/orders/"+chi.URLParam(r, "id"), http.StatusMovedPermanently)
}
