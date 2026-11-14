package ui

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// marketResultsTarget is the id of the region the board swaps in place.
const marketResultsTarget = "md-results"

// marketSortValues is every ordering the board offers. Anything else in the URL
// is not an error, it is the default.
var marketSortValues = map[string]bool{
	"relevance": true, "discount_desc": true, "discount_asc": true,
	"price_asc": true, "price_desc": true, "newest": true, "oldest": true,
}

// parseMarketDiscountsFilter reads the board's query string. It is the one
// place the URL is interpreted: the full page, the results fragment and the
// pager's links all go through it, so they cannot disagree about what a URL
// means. A value that does not parse is ignored, never an error, so a stale
// bookmark opens the board instead of failing.
func parseMarketDiscountsFilter(r *http.Request) (compare.MarketDiscountsFilter, string) {
	qs := r.URL.Query()

	sortBy := strings.TrimSpace(qs.Get("sort"))
	if !marketSortValues[sortBy] {
		sortBy = "newest"
	}
	view := strings.TrimSpace(qs.Get("view"))
	if view != "grid" {
		view = "list"
	}
	page, _ := strconv.Atoi(qs.Get("page"))
	if page <= 0 {
		page = 1
	}
	limit := 24
	if l, err := strconv.Atoi(strings.TrimSpace(qs.Get("limit"))); err == nil && (l == 24 || l == 48 || l == 96) {
		limit = l
	}

	// The board is the same board for everyone who may read it: the temporary
	// warehouses the platform's own moderators uploaded. There is no
	// per-organization slice of it, and the filter carries nothing about the
	// caller's organization — the source clause is a constant, so nothing a
	// request sends can widen it to include a supplier's private upload.
	filter := compare.MarketDiscountsFilter{
		DateFrom: validFilterDate(qs.Get("date_from")),
		DateTo:   validFilterDate(qs.Get("date_to")),
		Query:    clipRunes(strings.TrimSpace(qs.Get("q")), compare.MarketSuggestMaxChars),
		Supplier: clipRunes(strings.TrimSpace(qs.Get("supplier")), 200),
		SortBy:   sortBy,
		Page:     page,
		Limit:    limit,
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(qs.Get("product")), 10, 64); err == nil && id > 0 {
		filter.ProductID = &id
	}
	filter.MinPrice = parseMarketPrice(qs.Get("min_price"))
	filter.MaxPrice = parseMarketPrice(qs.Get("max_price"))
	filter.MinDiscount = parseMarketDiscount(qs.Get("min_discount"))
	filter.MaxDiscount = parseMarketDiscount(qs.Get("max_discount"))
	return filter, view
}

func clipRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

func parseMarketPrice(raw string) *money.Amount {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	val, err := money.Parse(raw)
	if err != nil || val.IsNegative() {
		return nil
	}
	return &val
}

func parseMarketDiscount(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
		return nil
	}
	if val > 100 {
		val = 100
	}
	return &val
}

// marketBoardEntitled reports whether the actor may read the board: staff
// always, a vendor organization only when its plan includes the feature. A
// failed entitlement lookup counts as no.
func (h *UIHandler) marketBoardEntitled(ctx context.Context, actor authctx.Actor) bool {
	if actor.IsStaff || h.billSvc == nil {
		return true
	}
	allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureMarketDiscounts)
	return err == nil && allowed
}

// isMarketResultsRequest is true for a request that asks for the results region
// only (the page script sets both headers).
func isMarketResultsRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == marketResultsTarget
}

// MarketDiscountsPage renders market-wide approved discounts across all
// suppliers and warehouses: the whole page, or — for a request that targets the
// results region — just the results.
func (h *UIHandler) MarketDiscountsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// The same URL answers a page and a fragment, so a shared cache must tell
	// them apart.
	w.Header().Add("Vary", "HX-Request")
	w.Header().Add("Vary", "HX-Target")

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/market-discounts", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.discounts.vendors_only"))
		return
	}
	if !h.marketBoardEntitled(ctx, actor) {
		h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", i18n.T(lang, "compare.discounts.upgrade_required"))
		return
	}

	filter, view := parseMarketDiscountsFilter(r)

	var result *compare.MarketDiscountsResult
	if h.compareSvc != nil {
		res, err := h.compareSvc.ListMarketDiscounts(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "list market discounts error", "error", err)
			http.Error(w, i18n.T(lang, "market.search.unavailable"), http.StatusServiceUnavailable)
			return
		}
		result = res
	}
	if result == nil {
		result = &compare.MarketDiscountsResult{
			Items:              make([]*compare.MarketDiscountRow, 0),
			Page:               filter.Page,
			Limit:              filter.Limit,
			TotalPages:         1,
			TotalKnown:         true,
			AvailableSuppliers: make([]string, 0),
		}
	}

	if isMarketResultsRequest(r) {
		h.renderPage(ctx, w, "render market discounts results", pages.MarketDiscountsResults(lang, result, filter, view))
		return
	}
	h.renderPage(ctx, w, "render market discounts", pages.MarketDiscountsPage(lang, dir, actor, result, filter, view))
}

// MarketDiscountsSuggest answers the search box's dropdown
// (GET /market-discounts/suggest?q=...). It is an XHR, so a caller who may not
// read the board gets a bare status code — never a redirect, never data.
func (h *UIHandler) MarketDiscountsSuggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "private, no-store")

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if actor.IsCustomer() || !h.marketBoardEntitled(ctx, actor) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	lang, _ := h.localeAndDir(r)
	query := clipRunes(strings.Join(strings.Fields(r.URL.Query().Get("q")), " "), compare.MarketSuggestMaxChars)
	if utf8.RuneCountInString(query) < compare.MarketSuggestMinChars {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}

	var (
		items  []compare.MarketSuggestion
		failed bool
	)
	if h.compareSvc == nil {
		failed = true
	} else {
		var err error
		items, err = h.compareSvc.SuggestMarketDiscounts(database.AsSystem(ctx), query, compare.MarketSuggestMaxItems)
		if err != nil {
			failed = true
			if !errors.Is(err, compare.ErrSuggestUnavailable) {
				h.log.ErrorContext(ctx, "market suggestions", "error", err)
			}
		}
	}
	h.renderPage(ctx, w, "render market suggestions", pages.MarketSuggestions(lang, query, items, failed))
}
