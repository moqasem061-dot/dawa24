package catalog

import (
	"context"
	"testing"
)

type phraseSearchRepo struct {
	mockCatalogRepo
	byQuery map[string][]*Product
	queries []string
}

func (r *phraseSearchRepo) SearchProducts(_ context.Context, params SearchParams) ([]*Product, error) {
	r.queries = append(r.queries, params.Query)
	return r.byQuery[params.Query], nil
}

func TestSearchFallsBackToDistinctWordsWhenPhraseHasNoMatch(t *testing.T) {
	first := &Product{ID: 1}
	second := &Product{ID: 2}
	repo := &phraseSearchRepo{
		byQuery: map[string][]*Product{
			"ماكينة":  {first, second},
			"تراكواز": {first},
			"موس":     {first},
			"3":       {second},
		},
	}
	service := NewService(repo, nil)

	products, err := service.Search(context.Background(), SearchParams{
		Query: "اليس ماكينة تراكواز 3 موس",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(products) != 2 || products[0].ID != first.ID || products[1].ID != second.ID {
		t.Fatalf("Search() IDs = %v, want [1 2] ranked by matched words", productIDs(products))
	}
	wantQueries := []string{"اليس ماكينة تراكواز 3 موس", "اليس", "ماكينة", "تراكواز", "3", "موس"}
	if len(repo.queries) != len(wantQueries) {
		t.Fatalf("search queries = %v, want %v", repo.queries, wantQueries)
	}
	for index, want := range wantQueries {
		if repo.queries[index] != want {
			t.Fatalf("search query %d = %q, want %q", index, repo.queries[index], want)
		}
	}
}

func TestSearchFallbackPreservesPagination(t *testing.T) {
	first := &Product{ID: 1}
	second := &Product{ID: 2}
	third := &Product{ID: 3}
	repo := &phraseSearchRepo{byQuery: map[string][]*Product{
		"brand name": {},
		"brand":      {first, second},
		"name":       {first, second, third},
	}}
	service := NewService(repo, nil)

	products, err := service.Search(context.Background(), SearchParams{Query: "brand name", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(products) != 1 || products[0].ID != second.ID {
		t.Fatalf("Search() IDs = %v, want [%d] for page 2", productIDs(products), second.ID)
	}
}

func TestFallbackSearchTermsRetainsSingleDigitTokens(t *testing.T) {
	terms := fallbackSearchTerms("medicine 3 tablets")
	want := []string{"medicine", "3", "tablets"}
	if len(terms) != len(want) {
		t.Fatalf("fallbackSearchTerms() = %v, want %v", terms, want)
	}
	for index := range want {
		if terms[index] != want[index] {
			t.Fatalf("fallbackSearchTerms()[%d] = %q, want %q", index, terms[index], want[index])
		}
	}
}

func TestSearchDoesNotRunWordFallbackWhenPhraseMatches(t *testing.T) {
	matched := &Product{ID: 7}
	repo := &phraseSearchRepo{byQuery: map[string][]*Product{"اسم الصنف": {matched}}}
	service := NewService(repo, nil)

	products, err := service.Search(context.Background(), SearchParams{Query: "اسم الصنف"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(products) != 1 || products[0].ID != matched.ID {
		t.Fatalf("Search() = %v, want product %d", productIDs(products), matched.ID)
	}
	if len(repo.queries) != 1 {
		t.Fatalf("ran %d searches, want the exact phrase search only", len(repo.queries))
	}
}

func productIDs(products []*Product) []int64 {
	ids := make([]int64, 0, len(products))
	for _, product := range products {
		ids = append(ids, product.ID)
	}
	return ids
}
