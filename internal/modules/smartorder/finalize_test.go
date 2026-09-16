package smartorder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// stubRepo implements just enough of Repository for finalisation.
type stubRepo struct {
	Repository
	lines       []*Line
	selections  map[int64]*Selection
	candidates  map[int64]*Candidate
	finalized   int
	statusCalls []RunStatus
	failFinal   error
}

func (s *stubRepo) ListLines(_ context.Context, _ int64, _ LineFilter) ([]*Line, int, error) {
	return s.lines, len(s.lines), nil
}

func (s *stubRepo) ListSelectionsByRun(_ context.Context, _, _ int64) (map[int64]*Selection, error) {
	return s.selections, nil
}

func (s *stubRepo) GetSelection(_ context.Context, _, lineID int64) (*Selection, error) {
	if sel, ok := s.selections[lineID]; ok {
		return sel, nil
	}
	return nil, errors.New("no selection")
}

func (s *stubRepo) GetCandidate(_ context.Context, _, candidateID int64) (*Candidate, error) {
	if c, ok := s.candidates[candidateID]; ok {
		return c, nil
	}
	return nil, errors.New("no candidate")
}

func (s *stubRepo) UpdateRunStatus(_ context.Context, _ int64, status RunStatus, _ int, _ string) error {
	s.statusCalls = append(s.statusCalls, status)
	return nil
}

func (s *stubRepo) FinalizeRun(_ context.Context, _, _ int64) error {
	if s.failFinal != nil {
		return s.failFinal
	}
	s.finalized++
	return nil
}

type stubPlacer struct {
	calls   int
	orderID int64
	err     error
}

func (p *stubPlacer) PlaceOrder(_ context.Context, _ PlaceOrderRequest) (int64, error) {
	p.calls++
	if p.err != nil {
		return 0, p.err
	}
	return p.orderID, nil
}

type stubRecheck struct {
	ok     bool
	reason IneligibleReason
}

func (r stubRecheck) Recheck(_ context.Context, _, _ int64, _ Candidate, _ float64) (bool, IneligibleReason, error) {
	return r.ok, r.reason, nil
}

func readyRun() *Run {
	return &Run{ID: 1, RunNumber: "SO-2026-000100", OrganizationID: 50, UserID: 7,
		BranchID: 3, Status: StatusCompleted, CurrentStep: 4}
}

func oneLineSetup() *stubRepo {
	return &stubRepo{
		lines: []*Line{{ID: 100, RawName: "بانادول", EffectiveQty: 10, Outcome: OutcomeOrdered}},
		selections: map[int64]*Selection{
			100: {LineID: 100, CandidateID: 900, LineNet: money.MustParse("100.00")},
		},
		candidates: map[int64]*Candidate{
			900: {ID: 900, LineID: 100, VariantID: 55, VendorOrgID: 51,
				Price: money.MustParse("12.00"), NetUnitPrice: money.MustParse("10.00"),
				Eligible: true},
		},
	}
}

func TestFinalizePlacesTheOrder(t *testing.T) {
	repo := oneLineSetup()
	placer := &stubPlacer{orderID: 4242}
	f := NewFinalizer(repo, placer, stubRecheck{ok: true})

	orderID, stale, err := f.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected no stale lines, got %d", len(stale))
	}
	if orderID != 4242 {
		t.Fatalf("expected order 4242, got %d", orderID)
	}
	if repo.finalized != 1 {
		t.Fatalf("expected exactly one finalisation, got %d", repo.finalized)
	}
}

// FR-047 and US7 scenario 4. A line that went stale must never be dropped or
// substituted — the buyer is told and decides.
func TestStaleLineBlocksTheOrderAndIsNamed(t *testing.T) {
	repo := oneLineSetup()
	placer := &stubPlacer{orderID: 1}
	f := NewFinalizer(repo, placer, stubRecheck{ok: false, reason: ReasonStock})

	orderID, stale, err := f.Finalize(context.Background(), readyRun())
	if err != nil {
		t.Fatalf("a stale line is not an error, it is a result: %v", err)
	}
	if orderID != 0 {
		t.Fatal("no order may be placed while a line is stale")
	}
	if placer.calls != 0 {
		t.Fatal("commerce must not be called when a line is stale")
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale line, got %d", len(stale))
	}
	if stale[0].RawName != "بانادول" {
		t.Fatalf("the stale line must be named, got %q", stale[0].RawName)
	}
	if stale[0].Reason != ReasonStock {
		t.Fatalf("expected the stock reason, got %q", stale[0].Reason)
	}
	if stale[0].Detail.Get("ar") == "" {
		t.Fatal("the buyer needs an explanation, not just a code")
	}
}

func TestEveryStaleReasonHasAnExplanation(t *testing.T) {
	for _, r := range []IneligibleReason{
		ReasonCoverage, ReasonStock, ReasonMinQty,
		ReasonInstitutional, ReasonInactive, ReasonOwnOrg,
	} {
		// Arabic is the platform's primary language and the one this text is
		// rendered in; English is checked too so the API answer is never a
		// half-filled bilingual value.
		if staleDetail(r).Get("ar") == "" {
			t.Errorf("%q has no Arabic explanation for the buyer", r)
		}
		if staleDetail(r).Get("en") == "" {
			t.Errorf("%q has no English explanation", r)
		}
	}
}

// FR-050. The failure that costs a pharmacy real money.
func TestAlreadyFinalizedRunIsRefused(t *testing.T) {
	repo := oneLineSetup()
	placer := &stubPlacer{orderID: 1}
	f := NewFinalizer(repo, placer, stubRecheck{ok: true})

	now := time.Now()
	run := readyRun()
	run.Status = StatusPlaced
	run.FinalizedAt = &now

	if _, _, err := f.Finalize(context.Background(), run); err == nil {
		t.Fatal("a run that already produced an order must be refused")
	}
	if placer.calls != 0 {
		t.Fatal("commerce must not be called for an already-finalised run")
	}
}

func TestStaleRunIsRefusedUntilRerun(t *testing.T) {
	repo := oneLineSetup()
	f := NewFinalizer(repo, &stubPlacer{}, stubRecheck{ok: true})

	run := readyRun()
	run.Status = StatusStale

	if _, _, err := f.Finalize(context.Background(), run); err == nil {
		t.Fatal("a stale run must be re-run before it can be finalised")
	}
}

func TestNothingOrderableIsRefusedWithAnExplanation(t *testing.T) {
	repo := &stubRepo{} // no lines
	f := NewFinalizer(repo, &stubPlacer{}, stubRecheck{ok: true})

	_, _, err := f.Finalize(context.Background(), readyRun())
	if err == nil {
		t.Fatal("an order with no orderable line must be refused")
	}
}

// A failure inside commerce must not strand the run in `finalizing` with no
// order and no way back.
func TestOrderCreationFailureReturnsTheRunToReview(t *testing.T) {
	repo := oneLineSetup()
	placer := &stubPlacer{err: errors.New("checkout unavailable")}
	f := NewFinalizer(repo, placer, stubRecheck{ok: true})

	if _, _, err := f.Finalize(context.Background(), readyRun()); err == nil {
		t.Fatal("expected the checkout failure to surface")
	}
	if repo.finalized != 0 {
		t.Fatal("no run may be marked finalised when no order was created")
	}

	last := repo.statusCalls[len(repo.statusCalls)-1]
	if last != StatusCompleted {
		t.Fatalf("the run should return to completed so the buyer can retry, got %s", last)
	}
}

func TestLineTotalsAreExact(t *testing.T) {
	repo := oneLineSetup()
	// 10 units at exactly 10.00 net.
	placer := &stubPlacer{orderID: 1}
	var captured PlaceOrderRequest
	f := NewFinalizer(repo, placerFunc(func(req PlaceOrderRequest) (int64, error) {
		captured = req
		return placer.orderID, nil
	}), stubRecheck{ok: true})

	if _, _, err := f.Finalize(context.Background(), readyRun()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Total.String() != "100.00" {
		t.Fatalf("expected exactly 100.00, got %s", captured.Total.String())
	}
	if len(captured.Lines) != 1 || captured.Lines[0].LineNet.String() != "100.00" {
		t.Fatalf("expected one line at 100.00, got %+v", captured.Lines)
	}
}

// placerFunc adapts a function to OrderPlacer.
type placerFunc func(PlaceOrderRequest) (int64, error)

func (p placerFunc) PlaceOrder(_ context.Context, req PlaceOrderRequest) (int64, error) {
	return p(req)
}
