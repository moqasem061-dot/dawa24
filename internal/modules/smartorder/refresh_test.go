package smartorder

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

// refreshRepo keeps a run's lines, candidates and selections in memory.
type refreshRepo struct {
	correctionRepo
	run        *Run
	lines      []*Line
	offers     []Offer
	cands      map[int64]*Candidate
	selections map[int64]*Selection
	nextID     int64
}

func (r *refreshRepo) GetRunByID(context.Context, int64, int64) (*Run, error) { return r.run, nil }
func (r *refreshRepo) ListLines(_ context.Context, _ int64, f LineFilter) ([]*Line, int, error) {
	var out []*Line
	for _, l := range r.lines {
		if f.Outcome == "" || string(l.Outcome) == f.Outcome {
			out = append(out, l)
		}
	}
	return out, len(out), nil
}
func (r *refreshRepo) LoadOffers(context.Context, int64, []int64) ([]Offer, error) {
	return r.offers, nil
}
func (r *refreshRepo) ReplaceRunCandidates(_ context.Context, _ int64, byLine map[int64][]Candidate) (map[int64][]Candidate, error) {
	r.cands = map[int64]*Candidate{}
	r.selections = map[int64]*Selection{} // ON DELETE CASCADE
	out := map[int64][]Candidate{}
	for lineID, cs := range byLine {
		for _, c := range cs {
			r.nextID++
			c.ID = r.nextID
			cc := c
			r.cands[c.ID] = &cc
			out[lineID] = append(out[lineID], c)
		}
	}
	return out, nil
}
func (r *refreshRepo) ListSelectionsByRun(context.Context, int64, int64) (map[int64]*Selection, error) {
	return r.selections, nil
}
func (r *refreshRepo) GetCandidate(_ context.Context, _, id int64) (*Candidate, error) {
	return r.cands[id], nil
}
func (r *refreshRepo) UpsertSelections(_ context.Context, sels []*Selection) error {
	for _, s := range sels {
		r.selections[s.LineID] = s
	}
	return nil
}

// The situation this exists for: a run processed while a supplier covered the
// buyer, opened after it stopped. The refresh must drop that supplier, keep the
// buyer's own pick where it still can deliver, and leave removed lines alone.
func TestRefreshFollowsTheWorldAndKeepsTheBuyersChoice(t *testing.T) {
	p5, p6 := int64(5), int64(6)
	repo := &refreshRepo{
		run: &Run{ID: 1, OrganizationID: 1, BranchID: 3, Status: StatusCompleted},
		lines: []*Line{
			{ID: 10, RunID: 1, MatchedProductID: &p5, EffectiveQty: 2, Outcome: OutcomeOrdered},
			{ID: 11, RunID: 1, MatchedProductID: &p6, EffectiveQty: 2, Outcome: OutcomeOrdered},
			{ID: 12, RunID: 1, MatchedProductID: &p5, EffectiveQty: 2, Outcome: OutcomeRemoved},
		},
		offers: []Offer{
			{ProductID: 5, VariantID: 50, VendorOrgID: 7, PriceMinor: 1000, StockQty: 9}, // Cairo vendor
			{ProductID: 5, VariantID: 51, VendorOrgID: 8, PriceMinor: 1200, StockQty: 9}, // Giza vendor
			{ProductID: 6, VariantID: 60, VendorOrgID: 7, PriceMinor: 3000, StockQty: 9},
			{ProductID: 6, VariantID: 61, VendorOrgID: 8, PriceMinor: 2000, StockQty: 9},
		},
		cands:      map[int64]*Candidate{900: {ID: 900, LineID: 11, VariantID: 60}},
		selections: map[int64]*Selection{11: {LineID: 11, CandidateID: 900, UserOverridden: true}},
		nextID:     1000,
	}
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetAvailabilityGate(fixedGate{verdicts: map[int64]GateVerdict{
		50: {Allowed: false, Reason: "not_covered"}, // stopped covering
		51: {Allowed: true},
		60: {Allowed: true}, // the buyer's pick, dearer, still deliverable
		61: {Allowed: true},
	}})

	if err := svc.Refresh(context.Background(), repo.run); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	chosen := func(line int64) int64 {
		sel := repo.selections[line]
		if sel == nil {
			return 0
		}
		return repo.cands[sel.CandidateID].VariantID
	}
	if got := chosen(10); got != 51 {
		t.Errorf("line 10 supplied by variant %d, want 51 (the uncovered vendor must drop out)", got)
	}
	if got := chosen(11); got != 60 {
		t.Errorf("line 11 supplied by variant %d, want the buyer's own pick 60", got)
	}
	if repo.lines[2].Outcome != OutcomeRemoved || repo.selections[12] != nil {
		t.Errorf("a removed line was revived: outcome %s", repo.lines[2].Outcome)
	}
	for _, c := range repo.cands {
		if c.VariantID == 50 && c.Eligible {
			t.Error("the vendor that stopped covering is still offered as eligible")
		}
	}
}

func TestRefreshIsDueAfterTheIntervalOrAtMidnight(t *testing.T) {
	var tr refreshTracker
	start := time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC) // 23:00 Cairo
	if !tr.due(1, start) {
		t.Fatal("a run never refreshed must be due")
	}
	tr.mark(1, start)
	if tr.due(1, start.Add(time.Minute)) {
		t.Error("refreshed a minute ago, should not be due")
	}
	if !tr.due(1, start.Add(refreshInterval)) {
		t.Error("should be due after the interval")
	}
	tr.mark(1, start.Add(50*time.Minute))      // 23:50 Cairo
	if !tr.due(1, start.Add(61*time.Minute)) { // 00:01 Cairo, eleven minutes later
		t.Error("a new Cairo day changes the delivery schedule and must be due")
	}
}
