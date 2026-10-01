package matchflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestSimulator_Modes(t *testing.T) {
	batch := matchflow.Batch{
		Catalog: []matchflow.CatalogEntry{
			{ProductID: 101, NameAR: "منتج 1"},
		},
		Items: []matchflow.Item{
			{Ref: 1, Text: "منتج 1", Options: []int64{101}},
			{Ref: 2, Text: "منتج 2", Options: []int64{102}},
		},
	}

	t.Run("Fast", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeFast)
		ds, err := sim.Enhance(context.Background(), batch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ds) != 2 {
			t.Fatalf("expected 2 decisions, got %d", len(ds))
		}
		if ds[0].ProductID == nil || *ds[0].ProductID != 101 {
			t.Errorf("expected product 101, got %v", ds[0].ProductID)
		}
	})

	t.Run("Down", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeDown)
		_, err := sim.Enhance(context.Background(), batch)
		if !errors.Is(err, matchflow.ErrSimulatorDown) {
			t.Errorf("expected ErrSimulatorDown, got %v", err)
		}
	})

	t.Run("Budget", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeBudget)
		_, err := sim.Enhance(context.Background(), batch)
		if !errors.Is(err, matchflow.ErrSimulatorBudget) {
			t.Errorf("expected ErrSimulatorBudget, got %v", err)
		}
	})

	t.Run("Malformed", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeMalformed)
		_, err := sim.Enhance(context.Background(), batch)
		if !errors.Is(err, matchflow.ErrSimulatorMalformed) {
			t.Errorf("expected ErrSimulatorMalformed, got %v", err)
		}
	})

	t.Run("Slow_TimesOut", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeSlow)
		sim.SlowDelay = 100 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := sim.Enhance(ctx, batch)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})

	t.Run("DuplicateRefs", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeDuplicateRefs)
		ds, err := sim.Enhance(context.Background(), batch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ds) != 4 {
			t.Fatalf("expected 4 decisions (2 per item), got %d", len(ds))
		}
		if ds[0].Ref != ds[1].Ref {
			t.Errorf("expected duplicate refs for first item, got %d and %d", ds[0].Ref, ds[1].Ref)
		}
	})

	t.Run("NotOffered", func(t *testing.T) {
		sim := matchflow.NewSimulator(matchflow.SimulatorModeNotOffered)
		ds, err := sim.Enhance(context.Background(), batch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ds) != 2 {
			t.Fatalf("expected 2 decisions, got %d", len(ds))
		}
		for _, d := range ds {
			if d.ProductID == nil || *d.ProductID != 999999999 {
				t.Errorf("expected not offered id 999999999, got %v", d.ProductID)
			}
		}
	})
}
