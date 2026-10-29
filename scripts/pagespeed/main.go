package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/ui"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8097", "listen address")
	flag.Parse()
	r := chi.NewRouter()
	r.Use(middleware.Compress(5))
	r.Use(httpx.SecurityHeaders(httpx.SecurityOptions{}))
	ui.RegisterStaticRoutes(r, func(*http.Request) string { return "http://" + *addr })
	r.Get("/review", reviewFixture)
	r.Get("/vendor/inventory/alerts", stockAlertsFixture)
	r.Get("/vendor/branches", branchMapFixture)
	r.Get("/market-discounts", marketFixture)
	r.Get("/admin/organizations", adminOrganizationsFixture)
	r.Get("/admin/finance/deposits", financeDepositsFixture)
	r.Get("/vendor/products/new", vendorPricingFixture)
	r.Get("/product", productFixture)
	r.Get("/documents", documentsFixture)
	r.Get("/vendor/documents", documentsFixture)
	r.Get("/vendor-dashboard", vendorDashboardFixture)
	r.Get("/admin/organizations/{id}/documents", documentsFixture)
	r.Get("/activities", activitiesFixture)
	r.Get("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.LoginPage("ar", "rtl", "", "").Render(layouts.WithPath(req.Context(), req.URL.Path), w); err != nil {
			log.Print(err)
		}
	})
	r.Get("/about", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.AboutPage("ar", "rtl", pages.AboutPageData{}).Render(layouts.WithPath(req.Context(), req.URL.Path), w); err != nil {
			log.Print(err)
		}
	})
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		ctx := layouts.WithPath(req.Context(), "/")
		lang, dir := "ar", "rtl"
		if req.URL.Query().Get("lang") == "en" {
			lang, dir = "en", "ltr"
		}
		if err := pages.CustomerHome(nil, nil, nil, pages.HomeStats{}, lang, dir).Render(ctx, w); err != nil {
			log.Print(err)
		}
	})
	log.Fatal(http.ListenAndServe(*addr, r))
}
