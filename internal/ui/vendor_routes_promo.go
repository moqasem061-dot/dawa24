package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerVendorPromoRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer.view"))
		g.Get("/vendor/offers", h.VendorOffersPage)
		g.Get("/vendor/offers/new", h.VendorOfferNewPage)
		g.Get("/vendor/offers/{id}/edit", h.VendorOfferEditPage)
		g.Get("/vendor/offers/{id}/locations", h.VendorOfferLocationsPage)
		g.Get("/vendor/offers/locations", h.VendorOffersLocationsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer.manage"))
		g.Post("/vendor/offers/new", h.VendorOfferNewSubmit)
		g.Post("/vendor/offers/{id}/edit", h.VendorOfferEditSubmit)
		g.Post("/vendor/offers/{id}/locations", h.VendorOfferLocationNewSubmit)
		g.Post("/vendor/offers/{id}/locations/new", h.VendorOfferLocationNewSubmit)
		g.Post("/vendor/offers/{id}/locations/{locId}/edit", h.VendorOfferLocationNewSubmit)
		g.Post("/vendor/offers/{id}/locations/{locId}/delete", h.VendorOfferLocationDeleteSubmit)
		g.Post("/vendor/offers/{id}/delete", h.VendorOfferDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer_package.view"))
		g.Get("/vendor/offers-packages", h.VendorOffersPackagesPage)
		g.Get("/vendor/offers-packages/{id}", h.VendorOffersPackagesPage)
		g.Get("/vendor/offers-packages/sponsorships", h.VendorOffersPackagesSponsorshipsPage)
		g.Get("/vendor/offers-packages/sponsorships/{id}", h.VendorOffersPackagesSponsorshipsPage)
		g.Get("/vendor/offers-packages/promotions", h.VendorOffersPackagesPromotionsPage)
		// كشف حساب للباقة: where this purchase's credits went. The card showed
		// "31 / 50" and nothing about the missing nineteen.
		g.Get("/vendor/offers-packages/purchases/{id}/statement", h.CreditStatementPage)
		g.Get("/vendor/sponsorship-requests", h.VendorSponsorshipRequestsPage)
		g.Post("/vendor/sponsorship-requests/new", h.VendorSponsorshipRequestSubmit)
		g.Post("/vendor/sponsorship-requests/{id}/cancel", h.VendorSponsorshipRequestCancelSubmit)
		g.Post("/vendor/sponsorship-packages/{id}/purchase", h.VendorSponsorshipPackagePurchaseSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ad.view"))
		g.Get("/vendor/ads", h.VendorAdsPage)
		g.Get("/vendor/ads/add", h.VendorAdsPage)
		g.Get("/vendor/ads/{id}", h.VendorAdsPage)
		g.Get("/vendor/ads/{id}/edit", h.VendorAdsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ad.manage"))
		g.Post("/vendor/ads/new", h.VendorAdCreateSubmit)
		g.Post("/vendor/ads/{id}/edit", h.VendorAdUpdateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.storefront.view"))
		g.Get("/vendor/storefront", h.VendorStorefrontPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.storefront.manage"))
		g.Post("/vendor/storefront/section", h.VendorStorefrontSectionSubmit)
		g.Post("/vendor/storefront/section/{id}/update", h.VendorStorefrontSectionUpdateSubmit)
		g.Post("/vendor/storefront/section/{id}/delete", h.VendorStorefrontSectionDeleteSubmit)
		g.Post("/vendor/storefront/section/{id}/toggle", h.VendorStorefrontSectionToggleSubmit)
	})
}
