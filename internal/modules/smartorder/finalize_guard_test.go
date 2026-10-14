package smartorder

import (
	"context"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestFinalizeRejectsCorruptDecisionBeforeOrderPlacement(t *testing.T) {
	repo := oneLineSetup()
	repo.lines[0].MatchDecision = []byte(`{"v":`)
	placer := &stubPlacer{orderID: 501}
	fin := NewFinalizer(repo, placer, stubRecheck{ok: true})
	fin.SetRevalidator(&mockMatchRevalidator{})
	orderID, _, err := fin.Finalize(context.Background(), readyRun())
	if err == nil || orderID != 0 || placer.calls != 0 {
		t.Fatalf("corrupt decision reached ordering: order=%d calls=%d error=%v", orderID, placer.calls, err)
	}
}

func TestSelectionRefusesLineAmountOverflow(t *testing.T) {
	s := &Service{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	line := &Line{ID: 1, EffectiveQty: 2}
	candidates := []Candidate{{ID: 1, VariantID: 10, Eligible: true, NetUnitPrice: money.FromMinor(math.MaxInt64)}}
	if selection := s.selectFor(&Config{Criteria: DefaultCriteria}, line, candidates, 10); selection != nil {
		t.Fatalf("overflow produced a priced selection: %+v", selection)
	}
}
