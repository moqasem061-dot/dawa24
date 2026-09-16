package catalog

import (
	"errors"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
)

// Progress for a long-running import.
//
// Preparing a nine-thousand-row file with AI enrichment is minutes of work, not
// milliseconds. Run inside the POST that started it, the admin gets a browser
// spinning on a request that may outlive its own timeout, with no way to tell a
// working import from a hung one. So preparation runs in the background and
// reports where it has reached, and the review screen polls for that.

// ImportPhase is the stage a preparation run has reached.
type ImportPhase string

const (
	// PhaseQueued means the run has not started yet.
	PhaseQueued ImportPhase = "queued"
	// ImportPhaseReading is decoding the workbook and finding its blocks.
	ImportPhaseReading ImportPhase = "reading"
	// ImportPhaseParsing is turning rows into products.
	ImportPhaseParsing ImportPhase = "parsing"
	// ImportPhaseMapping is the small AI translation pass: which column is which
	// field, and which catalogue value each of the file's distinct words means.
	ImportPhaseMapping ImportPhase = "mapping"
	// ImportPhaseMatching is resolving rows against the existing catalogue.
	ImportPhaseMatching ImportPhase = "matching"
	// ImportPhaseStaging is writing the staged rows for review.
	ImportPhaseStaging ImportPhase = "staging"
	// ImportPhaseDone means the review screen can be shown.
	ImportPhaseDone ImportPhase = "done"
	// ImportPhaseFailed means preparation stopped; nothing was written.
	ImportPhaseFailed ImportPhase = "failed"
)

// Label renders a phase for the progress panel.
func (p ImportPhase) Label() string {
	switch p {
	case ImportPhaseReading:
		return i18n.TDefault("w4_mod.s_274_274")
	case ImportPhaseParsing:
		return i18n.TDefault("w4_mod.s_275_275")
	case ImportPhaseMapping:
		return i18n.TDefault("w4_mod.s_276_276")
	case ImportPhaseMatching:
		return i18n.TDefault("w4_mod.s_277_277")
	case ImportPhaseStaging:
		return i18n.TDefault("w4_mod.s_278_278")
	case ImportPhaseDone:
		return i18n.TDefault("w4_mod.s_279_279")
	case ImportPhaseFailed:
		return i18n.TDefault("w4_mod.s_280_280")
	default:
		return i18n.TDefault("w4_mod.s_281_281")
	}
}

// Terminal reports whether nothing further will happen.
func (p ImportPhase) Terminal() bool {
	return p == ImportPhaseDone || p == ImportPhaseFailed
}

// ImportProgress is one snapshot of a preparation run.
type ImportProgress struct {
	Phase ImportPhase `json:"phase"`
	// Current and Total describe the work inside the phase. Total of zero means
	// the phase has no meaningful count, so the bar shows as indeterminate
	// rather than pretending to a precision it does not have.
	Current int `json:"current"`
	Total   int `json:"total"`
	// Message is the phase label, or the reason it failed.
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PhaseStartedAt is when the current phase began. A phase with no row count
	// drifts through its band on this, so a two-minute AI pass keeps the bar
	// moving instead of freezing it at the phase boundary.
	PhaseStartedAt time.Time `json:"phase_started_at"`
}

// phaseBands are each phase's contiguous share of the overall bar.
//
// Sized by how long each phase actually takes on a real administrative file,
// not evenly: reading a workbook is seconds, matching thirty-eight thousand
// rows against a twenty-thousand-product catalogue is the bulk of the run, and
// the AI mapping pass is the one that waits on a network. Weighting them
// equally produces a bar that sprints to 60% and then sits still, which is
// worse than no bar at all.
//
// They must be contiguous and end at 99; Percent supplies the last point only
// on the terminal phase.
var phaseBands = map[ImportPhase]importprogress.Band{
	PhaseQueued:         {Start: 0, End: 2},
	ImportPhaseReading:  {Start: 2, End: 10},
	ImportPhaseParsing:  {Start: 10, End: 28},
	ImportPhaseMapping:  {Start: 28, End: 46},
	ImportPhaseMatching: {Start: 46, End: 84},
	ImportPhaseStaging:  {Start: 84, End: 99},
}

// Percent is how far through the WHOLE run this snapshot sits, 0-100.
//
// It used to be how far through the current phase, which meant the bar ran
// 0→100 five times over and an administrator who looked away could not tell
// which pass they were watching. It also returned -1 for any phase without a
// row count — three of the six — so the bar spent most of a long import as an
// indeterminate barber-pole that discarded the one thing it did know.
func (p ImportProgress) Percent() int {
	switch p.Phase {
	case ImportPhaseDone:
		return importprogress.Complete
	case ImportPhaseFailed:
		// A failed run keeps whatever it had reached. Zeroing it implies the
		// work never started; filling it implies it finished.
		return importprogress.Percent(phaseBands[ImportPhaseStaging], 0, 0, 0)
	}
	band, ok := phaseBands[p.Phase]
	if !ok {
		return 0
	}
	return importprogress.Percent(band, p.Current, p.Total, p.PhaseElapsed())
}

// PhaseElapsed is how long the run has been in its current phase, which is what
// drives the drift for a phase that cannot count its own work.
func (p ImportProgress) PhaseElapsed() time.Duration {
	if p.PhaseStartedAt.IsZero() {
		return p.Elapsed()
	}
	return p.UpdatedAt.Sub(p.PhaseStartedAt)
}

// Elapsed is how long the run has been going.
func (p ImportProgress) Elapsed() time.Duration {
	if p.StartedAt.IsZero() {
		return 0
	}
	return p.UpdatedAt.Sub(p.StartedAt)
}

// ProgressFunc receives progress updates. A nil ProgressFunc is valid and
// ignores them, so the synchronous path costs nothing.
type ProgressFunc func(phase ImportPhase, current, total int)

func (f ProgressFunc) report(phase ImportPhase, current, total int) {
	if f != nil {
		f(phase, current, total)
	}
}

// ProgressTracker holds the live state of the preparation runs in flight.
//
// It is memory-only and per-process on purpose. Progress is a view of work
// happening in this process right now; persisting it would add a write per
// batch to say something that stops being true the moment the process ends. The
// durable record is the session row, which reaches "ready" when the work lands.
type ProgressTracker struct {
	mu   sync.RWMutex
	runs map[string]ImportProgress
}

// NewProgressTracker creates an empty tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{runs: map[string]ImportProgress{}}
}

// TryBegin registers a run only when no other run holds the session, returning
// false instead of overwriting one in flight. Check and claim happen under one
// lock acquisition: the previous Running()-then-Begin() sequence had a gap two
// simultaneous submits could both pass, each spawning a prepare against the
// same session.
func (t *ProgressTracker) TryBegin(sessionID string) (ProgressFunc, bool) {
	if t == nil {
		return nil, true
	}
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	if run, exists := t.runs[sessionID]; exists && !run.Phase.Terminal() {
		return nil, false
	}
	t.runs[sessionID] = ImportProgress{
		Phase: ImportPhaseReading, Message: ImportPhaseReading.Label(),
		StartedAt: now, UpdatedAt: now, PhaseStartedAt: now,
	}

	return func(phase ImportPhase, current, total int) {
		t.mu.Lock()
		defer t.mu.Unlock()
		run := t.runs[sessionID]
		if run.Phase != phase {
			run.PhaseStartedAt = time.Now()
		}
		run.Phase, run.Current, run.Total = phase, current, total
		run.Message = phase.Label()
		run.UpdatedAt = time.Now()
		if run.StartedAt.IsZero() {
			run.StartedAt = run.UpdatedAt
		}
		if run.PhaseStartedAt.IsZero() {
			run.PhaseStartedAt = run.UpdatedAt
		}
		t.runs[sessionID] = run
	}, true
}

// Finish marks a run complete, or failed with a reason.
func (t *ProgressTracker) Finish(sessionID string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	run := t.runs[sessionID]
	run.UpdatedAt = time.Now()
	if err != nil {
		run.Phase, run.Message = ImportPhaseFailed, err.Error()
	} else {
		run.Phase, run.Message = ImportPhaseDone, ImportPhaseDone.Label()
	}
	t.runs[sessionID] = run
	t.sweepLocked()
}

// Progress reads a run's latest snapshot.
func (t *ProgressTracker) Progress(sessionID string) (ImportProgress, bool) {
	if t == nil {
		return ImportProgress{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	run, ok := t.runs[sessionID]
	return run, ok
}

// Running reports whether a run is still in flight.
func (t *ProgressTracker) Running(sessionID string) bool {
	run, ok := t.Progress(sessionID)
	return ok && !run.Phase.Terminal()
}

// progressRetention is how long a finished run stays readable, so the page that
// polls it sees the completion rather than the entry vanishing under it.
const progressRetention = 10 * time.Minute

// sweepLocked drops finished runs that nobody is going to ask about again.
func (t *ProgressTracker) sweepLocked() {
	for id, run := range t.runs {
		if run.Phase.Terminal() && time.Since(run.UpdatedAt) > progressRetention {
			delete(t.runs, id)
		}
	}
}

// AI failure conditions a caller can act on differently.
//
// They are declared here rather than reused from the Gateway package because
// the catalogue must not import a transport: the adapter translates the
// Gateway's errors into these, and the import decides what to tell the admin.
var (
	// ErrAIQuotaExceeded means the platform's AI budget is spent. It is a
	// business condition an operator can fix, not an outage.
	ErrAIQuotaExceeded = errors.New("catalog: ai quota exceeded")
	// ErrAIUnauthorized means the Gateway rejected the key.
	ErrAIUnauthorized = errors.New("catalog: ai credentials rejected")
)
