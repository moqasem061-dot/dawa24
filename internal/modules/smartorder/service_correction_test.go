package smartorder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// correctionRepo is the slice of Repository CorrectMatch touches.
type correctionRepo struct {
	Repository
	line       *Line
	offers     []Offer
	candidates []Candidate
}

func (r *correctionRepo) GetLine(context.Context, int64, int64) (*Line, error) { return r.line, nil }
func (r *correctionRepo) ProductNames(context.Context, []int64) (map[int64]string, error) {
	return nil, nil
}
func (r *correctionRepo) LoadOffers(context.Context, int64, []int64) ([]Offer, error) {
	return r.offers, nil
}
func (r *correctionRepo) GetRunByID(context.Context, int64, int64) (*Run, error) {
	return &Run{ID: 1, BranchID: 3}, nil
}
func (r *correctionRepo) ReplaceCandidates(_ context.Context, _ int64, c []Candidate) error {
	r.candidates = c
	return nil
}
func (r *correctionRepo) ListCandidates(context.Context, int64, int64) ([]Candidate, error) {
	return r.candidates, nil
}
func (r *correctionRepo) GetConfig(context.Context, int64) (*Config, error)    { return nil, nil }
func (r *correctionRepo) UpsertSelections(context.Context, []*Selection) error { return nil }
func (r *correctionRepo) DeleteSelection(context.Context, int64, int64) error  { return nil }
func (r *correctionRepo) UpdateLines(context.Context, []*Line) error           { return nil }
func (r *correctionRepo) ListLines(context.Context, int64, LineFilter) ([]*Line, int, error) {
	return nil, 0, nil
}

type fixedGate struct {
	verdicts map[int64]GateVerdict
	err      error
}

func (g fixedGate) Check(context.Context, int64, int64, time.Time, []GateLine) (map[int64]GateVerdict, error) {
	return g.verdicts, g.err
}

// A corrected line used to fall back to the offer's own attributes whenever
// the gate was missing, failed, or had no verdict — and none of those know
// about coverage. An in-stock supplier that does not deliver to the buyer's
// branch then showed as orderable in the smart order while the catalogue,
// correctly, hid it.
func TestCorrectMatchNeverAdmitsAnOfferWithoutAVerdict(t *testing.T) {
	offer := Offer{ProductID: 5, VariantID: 50, VendorOrgID: 7, PriceMinor: 1000,
		StockQty: 100, VendorActive: true, ProductActive: true}

	cases := map[string]AvailabilityGate{
		"no gate":         nil,
		"gate error":      fixedGate{err: errors.New("database down")},
		"missing verdict": fixedGate{verdicts: map[int64]GateVerdict{}},
		"not covered":     fixedGate{verdicts: map[int64]GateVerdict{50: {Allowed: false, Reason: "not_covered"}}},
	}
	for name, gate := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &correctionRepo{line: &Line{ID: 9, RunID: 1, EffectiveQty: 2}, offers: []Offer{offer}}
			svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if gate != nil {
				svc.SetAvailabilityGate(gate)
			}
			if _, err := svc.CorrectMatch(context.Background(), 1, 9, 5); err != nil {
				t.Fatalf("CorrectMatch: %v", err)
			}
			if len(repo.candidates) != 1 || repo.candidates[0].Eligible {
				t.Fatalf("offer admitted without a covering verdict: %+v", repo.candidates)
			}
		})
	}

	t.Run("allowed", func(t *testing.T) {
		repo := &correctionRepo{line: &Line{ID: 9, RunID: 1, EffectiveQty: 2}, offers: []Offer{offer}}
		svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
		svc.SetAvailabilityGate(fixedGate{verdicts: map[int64]GateVerdict{50: {Allowed: true}}})
		if _, err := svc.CorrectMatch(context.Background(), 1, 9, 5); err != nil {
			t.Fatalf("CorrectMatch: %v", err)
		}
		if len(repo.candidates) != 1 || !repo.candidates[0].Eligible {
			t.Fatalf("an allowed offer was refused: %+v", repo.candidates)
		}
	})
}

func (r *correctionRepo) SaveLearnedMapping(context.Context, int64, string, int64) error  { return nil }
func (r *correctionRepo) UpdateRunStats(context.Context, *Run) error                      { return nil }
func (r *correctionRepo) SaveAlias(context.Context, int64, string, string, float64) error { return nil }
