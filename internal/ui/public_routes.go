package ui

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// RegisterPublicRoutes mounts everything a visitor may reach without signing
// in, wrapped only in OptionalAuth for the visitor analytics middleware.
// Rebuild V2 §1.3: these are the only routes without a forced audience.
func (h *UIHandler) RegisterPublicRoutes(r chi.Router) {
	if h.adminSvc != nil {
		DynamicRobotsTxtFetcher = func(ctx context.Context) (string, error) {
			settings, err := h.adminSvc.GetSEOSettings(ctx)
			if err != nil || settings == nil {
				return "", err
			}
			return settings.RobotsTxt, nil
		}
	}

	// Public routes take the visitor-analytics middleware and nothing else.
	// They are mounted through Group rather than r.Use: by the time this runs
	// the root mux already carries routes, and chi panics on a Use() after the
	// first route is defined. Group gives these routes their own middleware
	// stack without touching the parent, and without wrapping them in a gate.
	// Assets are served on their own group without the audience middlewares:
	// no session lookup, branch listing or settings query is worth running for
	// every CSS/JS/image request, and they are typically the majority of
	// traffic.
	r.Group(func(assets chi.Router) {
		RegisterStaticRoutes(assets)
		RegisterUploadRoutes(assets)
	})

	r.Group(func(pub chi.Router) {
		if h.idSvc != nil {
			pub.Use(identityHttp.OptionalAuth(h.idSvc, h.resolver, h.cookieName(), h.log))
		}
		pub.Use(h.BuyingBranchSelector)
		pub.Use(h.siteSettingsMiddleware)
		pub.Use(h.visitorMiddleware)
		pub.Use(h.LinkResponseHeadersMiddleware)
		pub.Use(h.MarkdownNegotiationMiddleware)

		// Sitemap & Agent Discovery metadata
		pub.Get("/sitemap.xml", h.SitemapXML)
		pub.Get("/llms.txt", h.LLMsTxt)
		pub.Get("/.well-known/agents-index.json", h.AgentsIndexJSON)

		// Public & Auth (marketing, catalogue browsing, sign-in, legal policies)
		pub.Get("/", h.HomePage)
		pub.Get("/privacy", h.PrivacyPage)
		pub.Get("/terms", h.TermsPage)
		pub.Get("/shipping-returns", h.ShippingReturnsPage)
		pub.Get("/refund", h.ShippingReturnsPage)
		pub.Get("/cookies", h.CookiesPolicyPage)
		pub.Get("/payment-policy", h.PaymentPolicyPage)
		pub.Get("/payments", h.PaymentPolicyPage)
		pub.Get("/policies/{slug}", h.DynamicPolicyPage)
		pub.Get("/vendor_agreement", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/terms", http.StatusMovedPermanently)
		})
		pub.Get("/about", h.AboutPage)
		pub.Get("/how-it-works", h.HowItWorksPage)
		pub.Get("/faq", h.FaqPage)
		pub.Get("/contact", h.ContactPage)
		pub.Get("/auth/login", h.LoginPage)
		pub.Get("/auth/mfa-verify", h.MFAVerifyPage)
		pub.Get("/auth/register", h.RegisterPage)
		pub.Get("/auth/forgot", h.ForgotPasswordPage)
		pub.Get("/auth/reset", h.ResetPasswordPage)
		pub.Get("/onboarding", h.OnboardingPage)
		pub.Get("/lang/{code}", h.SetLanguage)

		// The guarded routes: the catalogue listing and the supplier
		// directory, and nothing else.
		//
		// These two are the whole business asset in list form. /catalog
		// publishes supplier identity, net supply price, stock and expiry
		// across the market in one paginated view; /suppliers publishes who
		// every distributor is, where their branches are and what they cover.
		// Taken together they are the answer to "who sells what, at what
		// price, where" — which is the question this company exists to answer,
		// and the one a competitor would otherwise get for the cost of an
		// afternoon.
		//
		// The supplier profile is in the group with its listing: a directory is
		// only worth taking in bulk, and the profiles are the payload the
		// directory indexes.
		//
		// Guarded public marketplace listing routes
		pub.Group(func(guarded chi.Router) {
			guarded.Use(h.scrape.Protect)
			guarded.Get("/catalog", h.CustomerCatalogPage)
			guarded.Get("/suppliers", h.SuppliersPage)
			guarded.Get("/suppliers/{id}", h.SupplierProfilePage)
		})

		// Unguarded public routes
		pub.Get("/catalog/{id}", h.CustomerProductDetailPage)
		pub.Get("/offers", h.OffersPage)
		pub.Get("/offers/{id}", h.OfferDetailPage)
		pub.Get("/jobs", h.JobsPage)
		pub.Get("/jobs/{id}", h.JobDetailPage)
		pub.With(h.limiter.LimitByIP(60, time.Minute)).Get("/compare/search", h.CompareQuickSearch)
		pub.With(h.limiter.LimitByIP(60, time.Minute)).Get("/api/v1/compare/search", h.CompareQuickSearch)

		// The courier portal used to live here, unlisted and unauthenticated:
		// anyone holding a waybill number could read a pharmacy's address and
		// the cash due, and close the order. It is now إدارة الشحنات on the
		// supplier's own dashboard, where the parcel is shown to the
		// representative it was assigned to. The redirect keeps links a
		// supplier already shared from dead-ending; a signed-out visitor is
		// sent on to the login page by the vendor group's own gate.
		pub.Get("/delivery", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/delivery", http.StatusMovedPermanently)
		})
		pub.Get("/compare", h.ComparePlansPage)
		pub.Get("/compare/tool", h.CompareToolPage)
		pub.Get("/compare/sample", h.CompareSampleDownload)
		pub.Get("/compare/template", h.CompareSampleDownload)
		// Readiness of a freshly uploaded batch: the wizard waits on this
		// rather than opening a column mapping for a file nobody has parsed.
		pub.Get("/compare/files/staging", h.CompareStagingStatus)
		pub.Get("/compare/files/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/file/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/files/{id}/mapping-modal", h.CompareFileMappingModal)
		pub.Get("/compare/file/{id}/mapping-modal", h.CompareFileMappingModal)

		// State-changing compare mutations protected by CSRF (Audit P2-1 / Phase 3.4)
		pub.Group(func(comp chi.Router) {
			comp.Use(httpx.CSRF(h.secureCookie))
			comp.Post("/compare/subscribe", h.CompareSubscribeSubmit)
			comp.Post("/compare/upload", h.CompareUploadSubmit)
			comp.Post("/compare/files/{id}/rename", h.CompareFileRenameSubmit)
			comp.Post("/compare/file/{id}/rename", h.CompareFileRenameSubmit)
			comp.Post("/compare/files/{id}/archive", h.CompareFileArchiveSubmit)
			comp.Post("/compare/file/{id}/archive", h.CompareFileArchiveSubmit)
			comp.Post("/compare/files/{id}/unarchive", h.CompareFileUnarchiveSubmit)
			comp.Post("/compare/file/{id}/unarchive", h.CompareFileUnarchiveSubmit)
			comp.Post("/compare/files/{id}/delete", h.CompareFileDeleteSubmit)
			comp.Post("/compare/file/{id}/delete", h.CompareFileDeleteSubmit)
			comp.Post("/compare/files/{id}/skip", h.CompareFileSkipSubmit)
			comp.Post("/compare/file/{id}/skip", h.CompareFileSkipSubmit)
			comp.Post("/compare/files/{id}/mapping", h.CompareFileMappingSubmit)
			comp.Post("/compare/file/{id}/mapping", h.CompareFileMappingSubmit)
			comp.Post("/compare/rows/{id}/match", h.CompareRowManualMatchSubmit)
			comp.Post("/compare/run", h.CompareRunSubmit)
		})
		pub.Get("/compare/results", h.CompareResultsPage)
		pub.Get("/compare/head-to-head", h.CompareHeadToHeadPage)
		pub.Get("/compare/market-benchmark", h.CompareMarketBenchmarkPage)
		pub.Get("/compare/market-benchmark/offers", h.CompareBenchmarkOffersModal)
		pub.Get("/compare/market-benchmark/offers/close", h.CompareBenchmarkOffersClose)
		pub.Get("/compare/market-intelligence", h.CompareMarketIntelligencePage)
		pub.Get("/market-discounts", h.MarketDiscountsPage)
		pub.Get("/tracking", h.GuestOrderTrackingPage)
		pub.Get("/promotions/track-click/{offer}", h.PublicPromotionTrackClick)
		pub.Get("/promotions/track-click/{offer}/{promotion}", h.PublicPromotionTrackClick)
		pub.Get("/ads/click/{ad}", h.PublicAdClick)
		pub.Get("/ads/impression/{ad}", h.PublicAdImpression)

		// Form actions that work signed-out (sign-up must be reachable pre-login)
		if h.limiter != nil {
			pub.With(h.limiter.LimitByIP(10, time.Minute)).Post("/auth/login", h.LoginSubmit)
			pub.With(h.limiter.LimitByIP(10, time.Minute)).Post("/auth/mfa-verify", h.MFAVerifySubmit)
			pub.With(h.limiter.LimitByIP(5, time.Minute)).Post("/auth/register", h.RegisterSubmit)
			pub.With(h.limiter.LimitByIP(10, time.Minute)).Post("/auth/telegram/send-otp", h.SendTelegramOTP)
			pub.With(h.limiter.LimitByIP(20, time.Minute)).Post("/auth/telegram/verify-otp", h.VerifyTelegramOTP)
			pub.With(h.limiter.LimitByIP(5, time.Minute)).Post("/contact", h.ContactSubmit)
			pub.With(h.limiter.LimitByIP(10, time.Minute)).Post("/upload", h.UploadAPISubmit)
			pub.With(h.limiter.LimitByIP(10, time.Minute)).Post("/jobs/{id}/apply", h.JobApplySubmit)
			pub.With(h.limiter.LimitByIP(30, time.Minute)).Post("/offers/{id}/click", h.OfferClickSubmit)
		} else {
			pub.Post("/auth/login", h.LoginSubmit)
			pub.Post("/auth/mfa-verify", h.MFAVerifySubmit)
			pub.Post("/auth/register", h.RegisterSubmit)
			pub.Post("/auth/telegram/send-otp", h.SendTelegramOTP)
			pub.Post("/auth/telegram/verify-otp", h.VerifyTelegramOTP)
			pub.Post("/contact", h.ContactSubmit)
			pub.Post("/upload", h.UploadAPISubmit)
			pub.Post("/jobs/{id}/apply", h.JobApplySubmit)
			pub.Post("/offers/{id}/click", h.OfferClickSubmit)
		}
		pub.Post("/auth/logout", h.LogoutSubmit)
		pub.Get("/auth/logout", h.LogoutSubmit)
	})
}
