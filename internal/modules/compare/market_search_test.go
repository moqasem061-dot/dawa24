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
