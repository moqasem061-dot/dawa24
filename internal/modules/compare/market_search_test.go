package compare

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

type marketSearchStub struct {
	calls    int
	fail     bool
	deadline time.Time
}

func (s *marketSearchStub) SearchMarketCatalog(ctx context.Context, q string) ([]matchflow.CandidateView, error) {
	s.calls++
	s.deadline, _ = ctx.Deadline()
	if s.fail {
		return nil, errors.New("gateway unavailable")
	}
	return []matchflow.CandidateView{{ProductID: 7, Score: .95, Eligible: true}, {ProductID: 8, Score: .9, Eligible: false}, {ProductID: 9, Score: .1, Eligible: true}, {ProductID: 10, Score: .8, Eligible: true}}, nil
}
func TestMarketSearchCandidatesAndCache(t *testing.T) {
	stub := &marketSearchStub{}
	svc := NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetMarketCatalogSearcher(stub)
	svc.SetCache(newFakeCache())
	for range 2 {
		if got := svc.marketCatalogIDs(context.Background(), "بانادول"); !reflect.DeepEqual(got, []int64{7, 10}) {
			t.Fatalf("unsafe/irrelevant candidate IDs: %v", got)
		}
	}
	if stub.calls != 1 || stub.deadline.IsZero() || time.Until(stub.deadline) > 500*time.Millisecond {
		t.Fatalf("calls=%d deadline=%v", stub.calls, stub.deadline)
	}
}
func TestMarketSearchFailureFallsBack(t *testing.T) {
	stub := &marketSearchStub{fail: true}
	svc := NewService(&countingMarketRepo{total: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetMarketCatalogSearcher(stub)
	result, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Query: "Panadol"})
	if err != nil || result.TotalCount != 2 {
		t.Fatalf("text fallback failed: %v %+v", err, result)
	}
}
func TestMarketQueryBoundsUnicode(t *testing.T) {
	if got := marketQuery("  بانادول    ٥٠٠ "); got != "بانادول ٥٠٠" {
		t.Fatal(got)
	}
}

func TestMarketSearchUsesIndexedResultsWithoutFuzzyRetrieval(t *testing.T) {
	stub := &marketSearchStub{}
	svc := NewService(&countingMarketRepo{total: 2, rows: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetMarketCatalogSearcher(stub)
	_, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Query: "Panadol"})
	if err != nil || stub.calls != 0 {
		t.Fatalf("err=%v fuzzy calls=%d", err, stub.calls)
	}
}

type fuzzyFallbackRepo struct {
	countingMarketRepo
	listedIDs []int64
}

func (r *fuzzyFallbackRepo) CountMarketDiscounts(_ context.Context, filter MarketDiscountsFilter) (int64, error) {
	if len(filter.CatalogIDs) > 0 {
		return 3, nil
	}
	return 0, nil
}

// The indexed search found nothing, so the second pass runs with the catalogue's
// candidates — and the rows, which are now what decides whether that second pass
// happens, come back from it.
func (r *fuzzyFallbackRepo) ListMarketDiscounts(_ context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	r.listedIDs = filter.CatalogIDs
	if len(filter.CatalogIDs) == 0 {
		return &MarketDiscountsResult{Items: []*MarketDiscountRow{}, Page: 1, Limit: 24}, nil
	}
	return &MarketDiscountsResult{
		Items: []*MarketDiscountRow{{ID: 1}, {ID: 2}, {ID: 3}}, Page: 1, Limit: 24,
	}, nil
}

func TestMarketSearchFallbackUsesSameCandidatesForRowsAndCount(t *testing.T) {
	stub := &marketSearchStub{}
	repo := &fuzzyFallbackRepo{}
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetMarketCatalogSearcher(stub)
	result, err := svc.ListMarketDiscounts(context.Background(), MarketDiscountsFilter{Query: "pnadol"})
	if err != nil || result.TotalCount != 3 || stub.calls != 1 || !reflect.DeepEqual(repo.listedIDs, []int64{7, 10}) {
		t.Fatalf("err=%v result=%+v fuzzy calls=%d ids=%v", err, result, stub.calls, repo.listedIDs)
	}
}
