package ui

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// One parser serves the page, the results fragment and the pager's links, and a
// value that does not parse is dropped, never an error: a stale bookmark opens
// the board.
func TestParseMarketDiscountsFilterIgnoresInvalidValues(t *testing.T) {
	bad := url.Values{
		"product":      {"abc"},
		"sort":         {"drop table"},
		"view":         {"carousel"},
		"limit":        {"77"},
		"page":         {"-3"},
		"min_price":    {"-5"},
		"max_price":    {"cheap"},
		"min_discount": {"abc"},
		"max_discount": {"NaN"},
		"date_from":    {"2026-13-45"},
		"date_to":      {"tomorrow"},
	}
	filter, view := parseMarketDiscountsFilter(httptest.NewRequest("GET", "/market-discounts?"+bad.Encode(), nil))
	if filter.ProductID != nil || filter.MinPrice != nil || filter.MaxPrice != nil || filter.MinDiscount != nil || filter.MaxDiscount != nil {
		t.Errorf("an invalid value was accepted: %+v", filter)
	}
	if filter.SortBy != "newest" || view != "list" || filter.Limit != 24 || filter.Page != 1 {
		t.Errorf("defaults not applied: sort=%q view=%q limit=%d page=%d", filter.SortBy, view, filter.Limit, filter.Page)
	}
	if filter.DateFrom != "" || filter.DateTo != "" {
		t.Errorf("an invalid date was accepted: %q %q", filter.DateFrom, filter.DateTo)
	}

	for _, id := range []string{"0", "-4", "1.5", ""} {
		f, _ := parseMarketDiscountsFilter(httptest.NewRequest("GET", "/market-discounts?product="+id, nil))
		if f.ProductID != nil {
			t.Errorf("product=%q accepted as %d", id, *f.ProductID)
		}
	}
	for _, raw := range []string{"Inf", "+Inf", "-1"} {
		f, _ := parseMarketDiscountsFilter(httptest.NewRequest("GET", "/market-discounts?min_discount="+url.QueryEscape(raw), nil))
		if f.MinDiscount != nil {
			t.Errorf("min_discount=%q accepted as %v", raw, *f.MinDiscount)
		}
	}
}

func TestParseMarketDiscountsFilterAcceptsValidValues(t *testing.T) {
	long := strings.Repeat("ب", 300)
	q := url.Values{
		"product": {"7"}, "sort": {"discount_desc"}, "view": {"grid"}, "limit": {"48"}, "page": {"3"},
		"q": {"  " + long + "  "}, "supplier": {" مخزن "}, "min_price": {"10.50"}, "max_discount": {"250"},
		"date_from": {"2026-11-01"}, "date_to": {"2026-11-14"},
	}
	filter, view := parseMarketDiscountsFilter(httptest.NewRequest("GET", "/market-discounts?"+q.Encode(), nil))
	if filter.ProductID == nil || *filter.ProductID != 7 {
		t.Errorf("product = %v, want 7", filter.ProductID)
	}
	if filter.SortBy != "discount_desc" || view != "grid" || filter.Limit != 48 || filter.Page != 3 {
		t.Errorf("sort=%q view=%q limit=%d page=%d", filter.SortBy, view, filter.Limit, filter.Page)
	}
	if got := len([]rune(filter.Query)); got != 128 {
		t.Errorf("query kept %d characters, want it clipped to 128", got)
	}
	if filter.Supplier != "مخزن" {
		t.Errorf("supplier = %q, want it trimmed", filter.Supplier)
	}
	if filter.MinPrice == nil || filter.MinPrice.String() != "10.50" {
		t.Errorf("min price = %v", filter.MinPrice)
	}
	if filter.MaxDiscount == nil || *filter.MaxDiscount != 100 {
		t.Errorf("max discount = %v, want an over-range value capped at 100", filter.MaxDiscount)
	}
	if filter.DateFrom != "2026-11-01" || filter.DateTo != "2026-11-14" {
		t.Errorf("dates = %q %q", filter.DateFrom, filter.DateTo)
	}
}
