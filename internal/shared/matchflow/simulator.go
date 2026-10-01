package matchflow

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// SimulatorMode dictates how the simulated Enhancer responds.
type SimulatorMode string

const (
	SimulatorModeFast          SimulatorMode = "fast"
	SimulatorModeSlow          SimulatorMode = "slow"
	SimulatorModeDown          SimulatorMode = "down"
	SimulatorModeBudget        SimulatorMode = "budget"
	SimulatorModeMalformed     SimulatorMode = "malformed"
	SimulatorModeDuplicateRefs SimulatorMode = "duplicate_refs"
	SimulatorModeNotOffered    SimulatorMode = "not_offered"
)

var (
	// ErrSimulatorDown simulates an upstream gateway outage or HTTP 503.
	ErrSimulatorDown = errors.New("ai simulator: gateway unavailable (503)")
	// ErrSimulatorBudget simulates quota/budget exhaustion or HTTP 429.
	ErrSimulatorBudget = errors.New("ai simulator: budget or quota exhausted (429)")
	// ErrSimulatorMalformed simulates an unparseable or corrupted response.
	ErrSimulatorMalformed = errors.New("ai simulator: malformed response")
)

// Simulator implements Enhancer and Verifier for testing and CLI simulation.
type Simulator struct {
	Mode         SimulatorMode
	FastDelay    time.Duration
	SlowDelay    time.Duration
	NotOfferedID int64
	attempts     int64
	verifies     int64
}

// NewSimulator creates a new Simulator configured for the given mode.
func NewSimulator(mode SimulatorMode) *Simulator {
	return &Simulator{
		Mode:         mode,
		FastDelay:    0,
		SlowDelay:    20 * time.Second,
		NotOfferedID: 999999999,
	}
}

// Attempts returns the count of Enhance calls handled.
func (s *Simulator) Attempts() int {
	return int(atomic.LoadInt64(&s.attempts))
}

// Verifies returns the count of Verify calls handled.
func (s *Simulator) Verifies() int {
	return int(atomic.LoadInt64(&s.verifies))
}

// Enhance simulates answering a batch of items according to s.Mode.
func (s *Simulator) Enhance(ctx context.Context, batch Batch) ([]Decision, error) {
	atomic.AddInt64(&s.attempts, 1)

	switch s.Mode {
	case SimulatorModeDown:
		return nil, ErrSimulatorDown

	case SimulatorModeBudget:
		return nil, ErrSimulatorBudget

	case SimulatorModeMalformed:
		return nil, ErrSimulatorMalformed

	case SimulatorModeSlow:
		delay := s.SlowDelay
		if delay <= 0 {
			delay = 20 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			return s.fastDecisions(batch), nil
		}

	case SimulatorModeDuplicateRefs:
		if s.FastDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.FastDelay):
			}
		}
		var out []Decision
		for _, it := range batch.Items {
			id1 := s.pickOption(it, batch)
			id2 := id1
			if id1 != nil {
				val := *id1 + 1000000
				id2 = &val
			}
			out = append(out, Decision{
				Ref:        it.Ref,
				ProductID:  id1,
				Confidence: 0.95,
				Reason:     "simulated duplicate ref 1",
			})
			out = append(out, Decision{
				Ref:        it.Ref,
				ProductID:  id2,
				Confidence: 0.90,
				Reason:     "simulated duplicate ref 2 (differing content)",
			})
		}
		return out, nil

	case SimulatorModeNotOffered:
		if s.FastDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.FastDelay):
			}
		}
		out := make([]Decision, 0, len(batch.Items))
		badID := s.NotOfferedID
		if badID <= 0 {
			badID = 999999999
		}
		for _, it := range batch.Items {
			val := badID
			out = append(out, Decision{
				Ref:        it.Ref,
				ProductID:  &val,
				Confidence: 0.95,
				Reason:     "simulated not offered target",
			})
		}
		return out, nil

	case SimulatorModeFast:
		fallthrough
	default:
		if s.FastDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.FastDelay):
			}
		}
		return s.fastDecisions(batch), nil
	}
}

// Verify simulates pair verification according to s.Mode.
func (s *Simulator) Verify(ctx context.Context, batch CheckBatch) ([]Check, error) {
	atomic.AddInt64(&s.verifies, 1)

	switch s.Mode {
	case SimulatorModeDown:
		return nil, ErrSimulatorDown

	case SimulatorModeBudget:
		return nil, ErrSimulatorBudget

	case SimulatorModeMalformed:
		return nil, ErrSimulatorMalformed

	case SimulatorModeSlow:
		delay := s.SlowDelay
		if delay <= 0 {
			delay = 20 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			return s.fastChecks(batch), nil
		}

	case SimulatorModeFast:
		fallthrough
	default:
		if s.FastDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.FastDelay):
			}
		}
		return s.fastChecks(batch), nil
	}
}

func (s *Simulator) fastDecisions(batch Batch) []Decision {
	out := make([]Decision, 0, len(batch.Items))
	for _, it := range batch.Items {
		id := s.pickOption(it, batch)
		out = append(out, Decision{
			Ref:        it.Ref,
			ProductID:  id,
			Confidence: 0.95,
			Reason:     "matched by ai simulator",
		})
	}
	return out
}

func (s *Simulator) fastChecks(batch CheckBatch) []Check {
	out := make([]Check, 0, len(batch.Pairs))
	for _, p := range batch.Pairs {
		out = append(out, Check{
			Ref:        p.Ref,
			Brand:      AttrSame,
			Strength:   AttrSame,
			Form:       AttrSame,
			Variant:    AttrSame,
			Verdict:    CheckSame,
			Confidence: 0.95,
			Reason:     "verified by ai simulator",
		})
	}
	return out
}

func (s *Simulator) pickOption(it Item, batch Batch) *int64 {
	if len(it.Options) > 0 {
		val := it.Options[0]
		return &val
	}
	if it.CurrentGuess != nil {
		return it.CurrentGuess
	}
	if len(batch.Catalog) > 0 {
		val := batch.Catalog[0].ProductID
		return &val
	}
	return nil
}
