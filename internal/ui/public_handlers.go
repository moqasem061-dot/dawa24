package ui

import (
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var featured []*catalog.Product
	var categories []*catalog.Category
	var offers []*promo.Offer
	stats := pages.HomeStats{
		TotalSuppliers: 47,
		TotalProducts:  8340,
		TotalCities:    86,
		TotalOrders:    1420,
	}

	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 8})
		if err != nil {
			h.log.WarnContext(ctx, "home page: search featured products", "error", err)
		} else {
			featured = prods
			if len(prods) > 0 {
				stats.TotalProducts = 8340 + len(prods)
			}
		}

		cats, err := h.catSvc.ListCategories(ctx)
		if err != nil {
			h.log.WarnContext(ctx, "home page: list categories", "error", err)
		} else {
			categories = cats
		}
	}

	if h.promoSvc != nil {
		if activeOffers, err := h.promoSvc.ListActiveOffers(ctx, 4, 0); err == nil {
			offers = activeOffers
			stats.TotalOffers = len(activeOffers)
		}

		// The hero is a carousel of a handful of slides, and every slide it is
		// given is rendered into the DOM with its own Alpine transition and
		// intersection observer. Handing it every active ad on the platform is
		// what made the landing page lock the browser up; it is capped here, at
		// the source, so no amount of ad inventory can do that again.
		if allActiveAds, err := h.promoSvc.ListActiveAds(ctx, ""); err == nil && len(allActiveAds) > 0 {
			stats.Ads = capAds(shuffleAds(allActiveAds), maxHeroAds)
		} else {
			if heroAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeHero); err == nil {
				stats.Ads = append(stats.Ads, heroAds...)
			}
			if bannerAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeBanner); err == nil {
				stats.Ads = append(stats.Ads, bannerAds...)
			}
			stats.Ads = capAds(shuffleAds(stats.Ads), maxHeroAds)
		}
		if dealsAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeDeals); err == nil {
			stats.DealsAds = capAds(shuffleAds(dealsAds), maxSectionAds)
		}
		if bottomAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeBottom); err == nil {
			stats.BottomAds = capAds(shuffleAds(bottomAds), maxSectionAds)
		}
		// One batched enrichment for all three groups rather than three, so the
		// same supplier appearing in two of them is fetched once.
		h.enrichAds(ctx, concatAds(stats.Ads, stats.DealsAds, stats.BottomAds))
	}

	if h.orgSvc != nil {
		// COUNT(*), not "load a hundred rows and take the length".
		typ := org.TypeVendor
		if n, err := h.orgSvc.CountOrganizations(database.AsSystem(ctx), &typ, nil); err == nil && n > 0 {
			stats.TotalSuppliers = n
		}
	}

	if cities := h.listCities(ctx); len(cities) > 0 {
		stats.TotalCities = len(cities)
	}

	if h.adminSvc != nil {
		if b, err := h.adminSvc.GetContentBlockByKey(ctx, "home-hero"); err == nil && b != nil && b.IsActive {
			stats.HeroTitle = b.Title.Get(i18n.Lang(lang))
			stats.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
		}
	}

	h.renderPage(ctx, w, "render home page", pages.CustomerHome(featured, categories, offers, stats, lang, dir))
}

func (h *UIHandler) PrivacyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "privacy", "سياسة الخصوصية وحماية البيانات")
}

func (h *UIHandler) TermsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "terms", "شروط وأحكام الاستخدام")
}

func (h *UIHandler) ShippingReturnsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "shipping_return", "سياسة الشحن والتسليم والاسترجاع والإلغاء")
}

func (h *UIHandler) CookiesPolicyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "cookies", "سياسة ملفات تعريف الارتباط (Cookies)")
}

func (h *UIHandler) PaymentPolicyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "payment", "سياسة الدفع والتعاملات المالية")
}

func (h *UIHandler) DynamicPolicyPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.renderPolicy(w, r, slug, "السياسات والضوابط القانونية")
}

// How many sponsored ads the landing page will render. The hero is a carousel
// the reader steps through; the sections are strips. Both put every item they
// are given into the DOM at once, so these are the real budgets.
const (
	maxHeroAds    = 6
	maxSectionAds = 8
)

// capAds trims an ad list to at most n, preserving order.
func capAds(ads []*promo.Ad, n int) []*promo.Ad {
	if n <= 0 || len(ads) <= n {
		return ads
	}
	return ads[:n]
}

// shuffleAds reorders an ad list in place with Fisher-Yates, so every page
// refresh shows the ads in a different order instead of the same newest-first
// lineup. It runs after fetch and before capAds, which is what gives
// lower-ranked ads a fair chance at the capped slots: the query stays on its
// index (no ORDER BY RANDOM()) and admin/API orderings stay deterministic.
// Tracking is unaffected — rotation never changes ad IDs, links or the
// impression beacons rendered per ad.
func shuffleAds(ads []*promo.Ad) []*promo.Ad {
	if len(ads) < 2 {
		return ads
	}
	rand.Shuffle(len(ads), func(i, j int) {
		ads[i], ads[j] = ads[j], ads[i]
	})
	return ads
}

// concatAds joins the page's ad groups into one slice for a single enrichment
// pass. The groups share pointers with the originals, so enriching the joined
// slice enriches all three.
func concatAds(groups ...[]*promo.Ad) []*promo.Ad {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]*promo.Ad, 0, total)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
