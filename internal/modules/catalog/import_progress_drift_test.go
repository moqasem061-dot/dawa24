package catalog

import (
	"testing"
	"time"
)

// The reported symptom was a bar that stopped at one number in the taxonomy
// phase and never moved again until the run failed. The arithmetic below is why
// it could not move: a phase with no row count drifts on elapsed time, and
// elapsed time was measured from the snapshot's own UpdatedAt — which only
// advances when the run REPORTS. A phase that reports nothing for four minutes
// therefore had an elapsed time of zero for four minutes.
func TestUncountedPhaseDriftsWithTheClock(t *testing.T) {
	start := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	p := ImportProgress{
		Phase:          ImportPhaseMapping,
		StartedAt:      start,
		UpdatedAt:      start, // the run has reported once and gone quiet
		PhaseStartedAt: start,
	}

	atStart := p.PercentAt(start)
	band := phaseBands[ImportPhaseMapping]
	if atStart != int(band.Start) {
		t.Fatalf("at the phase boundary: got %d%%, want %d%%", atStart, int(band.Start))
	}

	later := p.PercentAt(start.Add(30 * time.Second))
	if later <= atStart {
		t.Fatalf("after 30s of silence the bar did not move: %d%% then %d%%", atStart, later)
	}
	if float64(later) >= band.End {
		t.Fatalf("drift reached the next phase's band: %d%% is not below %v", later, band.End)
	}

	// Monotonic, and never out of its own band however long the phase runs.
	verylate := p.PercentAt(start.Add(2 * time.Hour))
	if verylate < later {
		t.Fatalf("drift went backwards: %d%% then %d%%", later, verylate)
	}
	if float64(verylate) >= band.End {
		t.Fatalf("drift escaped its band after two hours: %d%%", verylate)
	}
}

// A phase that CAN count its work interpolates and must not drift past what it
// counted, however long it takes.
func TestCountedPhaseIgnoresTheClock(t *testing.T) {
	start := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	p := ImportProgress{
		Phase:          ImportPhaseMatching,
		Current:        500,
		Total:          1000,
		StartedAt:      start,
		UpdatedAt:      start,
		PhaseStartedAt: start,
	}

	early := p.PercentAt(start.Add(time.Second))
	late := p.PercentAt(start.Add(time.Hour))
	if early != late {
		t.Fatalf("a counted phase drifted: %d%% then %d%%", early, late)
	}
}

// A snapshot read back from the session row carries the phase's start, so the
// second tab drifts exactly like the first. Before this it carried only a
// single timestamp used as both start and last-update, and their difference —
// the elapsed time the drift ran on — was zero by construction.
func TestPersistedSnapshotDrifts(t *testing.T) {
	phaseBegan := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	p := ImportProgress{
		Phase:          ImportPhaseMapping,
		StartedAt:      phaseBegan,
		UpdatedAt:      phaseBegan,
		PhaseStartedAt: phaseBegan,
	}

	if got := p.PercentAt(phaseBegan.Add(time.Minute)); got <= int(phaseBands[ImportPhaseMapping].Start) {
		t.Fatalf("a row-sourced snapshot did not drift: %d%%", got)
	}
}

// Nothing but a terminal state may say 100.
func TestOnlyCompletionReachesOneHundred(t *testing.T) {
	start := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, phase := range []ImportPhase{
		ImportPhaseReading, ImportPhaseParsing, ImportPhaseMapping,
		ImportPhaseMatching, ImportPhaseStaging,
	} {
		p := ImportProgress{Phase: phase, StartedAt: start, PhaseStartedAt: start}
		if got := p.PercentAt(start.Add(24 * time.Hour)); got >= 100 {
			t.Errorf("phase %q reached %d%% by drifting", phase, got)
		}
	}
	done := ImportProgress{Phase: ImportPhaseDone}
	if got := done.Percent(); got != 100 {
		t.Errorf("a finished run reports %d%%, want 100", got)
	}
}
