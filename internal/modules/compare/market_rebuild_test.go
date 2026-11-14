package compare

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The header's "highest discount" describes the whole filtered board, so it
// comes from the same cached scan as the count, not from the page on show.
func TestMarketBoardCarriesTheBoardWideMaximumDiscount(t *testing.T) {
	repo := &countingMarketRepo{total: 500, rows: 24, maxDiscount: 63}
	svc := &Service{repo: repo}
	svc.SetCache(newFakeCache())

	for i := 0; i < 2; i++ {
		res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1 + i})
		if err != nil {
			t.Fatal(err)
		}
		if !res.TotalKnown || res.TotalCount != 500 || res.MaxDiscount != 63 {
			t.Fatalf("page %d: known=%v total=%d max=%v, want a known 500 rows and a maximum of 63", res.Page, res.TotalKnown, res.TotalCount, res.MaxDiscount)
		}
	}
	if repo.counts != 1 {
		t.Fatalf("counted %d times, want the maximum to share the count's cached scan", repo.counts)
	}
}

// When the count cannot be read the maximum is unknown too, and the page says
// so (TotalKnown) instead of printing the page's own best discount.
func TestMarketBoardMaximumIsUnknownWithoutACount(t *testing.T) {
	svc := &Service{repo: &countingMarketRepo{rows: 3, maxDiscount: 63, countErr: errors.New("budget exceeded")}}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalKnown || res.MaxDiscount != 0 {
		t.Fatalf("known=%v max=%v, want unknown with no invented maximum", res.TotalKnown, res.MaxDiscount)
	}
}

// A product filter is part of the count's identity: counting product 7 must
// never be answered with the count cached for product 9.
func TestMarketCountKeyIncludesTheProductFilter(t *testing.T) {
	seven, nine := int64(7), int64(9)
	a := marketFilterFingerprint(MarketDiscountsFilter{ProductID: &seven})
	b := marketFilterFingerprint(MarketDiscountsFilter{ProductID: &nine})
	none := marketFilterFingerprint(MarketDiscountsFilter{})
	if a == b || a == none || b == none {
		t.Fatalf("fingerprints collide: %s %s %s", a, b, none)
	}
}

type productNamingRepo struct {
	countingMarketRepo
	names int
}

func (r *productNamingRepo) MarketProductNames(context.Context, int64) (string, string, error) {
	r.names++
	return "بانادول", "Panadol", nil
}

func TestMarketBoardNamesTheFilteredProduct(t *testing.T) {
	id := int64(7)
	repo := &productNamingRepo{countingMarketRepo: countingMarketRepo{total: 0, rows: 0}}
	svc := &Service{repo: repo}
	res, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{ProductID: &id, Limit: 24, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProductNameAR != "بانادول" || res.ProductNameEN != "Panadol" {
		t.Fatalf("product names = %q / %q, want the lookup's names even on an empty page", res.ProductNameAR, res.ProductNameEN)
	}
	// No filter, no lookup.
	repo.names = 0
	if _, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Limit: 24, Page: 1}); err != nil {
		t.Fatal(err)
	}
	if repo.names != 0 {
		t.Fatalf("looked up a product name %d times without a product filter", repo.names)
	}
}

type suggestingRepo struct {
	countingMarketRepo
	calls   int
	gotQ    string
	gotMax  int
	lines   []MarketSuggestion
	failing bool
}

func (r *suggestingRepo) SuggestMarketDiscounts(_ context.Context, q string, limit int) ([]MarketSuggestion, error) {
	r.calls++
	r.gotQ, r.gotMax = q, limit
	if r.failing {
		return nil, errors.New("deadline exceeded")
	}
	return r.lines, nil
}

func TestSuggestMarketDiscounts(t *testing.T) {
	line := func(name string) MarketSuggestion {
		return MarketSuggestion{FileName: name, Offers: 1, Warehouses: 1, LowestPrice: money.MustParse("1.00")}
	}
	t.Run("a one-character query never reaches the database", func(t *testing.T) {
		repo := &suggestingRepo{lines: []MarketSuggestion{line("x")}}
		for _, q := range []string{"", "  ", "ب", " b "} {
			out, err := (&Service{repo: repo}).SuggestMarketDiscounts(context.Background(), q, 8)
			if err != nil || len(out) != 0 {
				t.Fatalf("q=%q: out=%v err=%v, want nothing", q, out, err)
			}
		}
		if repo.calls != 0 {
			t.Fatalf("database called %d times for queries under two characters", repo.calls)
		}
	})
	t.Run("two Arabic characters are enough, and the limit is capped at eight", func(t *testing.T) {
		var many []MarketSuggestion
		for i := 0; i < 20; i++ {
			many = append(many, line("n"))
		}
		repo := &suggestingRepo{lines: many}
		out, err := (&Service{repo: repo}).SuggestMarketDiscounts(context.Background(), "بن", 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != MarketSuggestMaxItems || repo.gotMax != MarketSuggestMaxItems {
			t.Fatalf("returned %d lines, asked for %d, want at most %d", len(out), repo.gotMax, MarketSuggestMaxItems)
		}
	})
	t.Run("a failure is the unavailable sentinel, never a raw error", func(t *testing.T) {
		repo := &suggestingRepo{failing: true}
		_, err := (&Service{repo: repo}).SuggestMarketDiscounts(context.Background(), "panadol", 8)
		if !errors.Is(err, ErrSuggestUnavailable) {
			t.Fatalf("err = %v, want ErrSuggestUnavailable", err)
		}
	})
	t.Run("a repository without suggestions is unavailable", func(t *testing.T) {
		_, err := (&Service{repo: &countingMarketRepo{}}).SuggestMarketDiscounts(context.Background(), "panadol", 8)
		if !errors.Is(err, ErrSuggestUnavailable) {
			t.Fatalf("err = %v, want ErrSuggestUnavailable", err)
		}
	})
}

func TestMarketSuggestionDisplayName(t *testing.T) {
	s := MarketSuggestion{NameAR: "بانادول", NameEN: "Panadol", FileName: "panadol 500 file"}
	if s.DisplayName("ar") != "بانادول" || s.DisplayName("en") != "Panadol" {
		t.Errorf("catalogue names by language: %q / %q", s.DisplayName("ar"), s.DisplayName("en"))
	}
	if (MarketSuggestion{NameAR: "بانادول", FileName: "f"}).DisplayName("en") != "بانادول" {
		t.Error("English reader with only an Arabic catalogue name must still see it")
	}
	if (MarketSuggestion{FileName: "from the file"}).DisplayName("ar") != "from the file" {
		t.Error("an unmatched line shows the file's own name")
	}
}
