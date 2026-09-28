package matchflow

// The engine every importer's AI stage runs on.
//
// Four tools asked the model the same question — the smart order, the vendor
// price list, the saving list and the master-catalogue import — and each had
// its own loop around the call. The loops had drifted in exactly the places
// that decide whether the AI helps at all:
//
//   - The per-request timeout was 20 seconds in one, 60 in another and absent
//     in a third, against a Gateway that gives this capability 150. A batch
//     of a hundred Arabic lines needs more than twenty seconds to answer, so
//     on the master-catalogue import the stage timed out on essentially every
//     request and reported "the AI found nothing".
//   - A failed batch was simply dropped. One malformed or truncated response
//     took two hundred rows back to their deterministic outcome, and nothing
//     asked again.
//   - An item the model skipped was silently unanswered.
//   - Nothing checked a proposal before applying it, so the forced match —
//     the model choosing the nearest thing on a list that did not contain the
//     answer — went straight into the file.
//
// Run is the one loop. Callers bring questions and apply answers; everything
// between — packing, concurrency, timeouts, splitting a failed batch,
// re-asking skipped items, the pair check, the wall clock — happens here, the
// same way for every tool.

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// requestTimeout bounds one call. Slightly above the Gateway's own 150 s for
// this capability, so the Gateway's classification of a slow upstream is the
// one that surfaces rather than a bare context error.
const requestTimeout = 170 * time.Second

// hardWallClock is the stage's absolute ceiling whatever a profile says. The
// product decision is that an import's AI stage finishes inside fifteen
// minutes; a minute is left for the caller to persist what came back.
const hardWallClock = 14 * time.Minute

// proposeShare is the part of the wall clock new proposal requests may start
// in. The rest is kept for the pair checks of requests already in flight: a
// proposal that cannot be verified is not applied, so a run that spent its
// whole budget proposing would have bought answers it then had to discard.
const proposeShare = 0.8

// splitMin is the smallest batch that is split rather than given up on.
const splitMin = 6

// maxDepth bounds re-asking: a failed batch is halved at most this many
// times, and skipped items are re-asked at most this many times.
const maxDepth = 3

// checkChunk is how many pairs one verification request carries. Pairs are
// small — one line and one product — so this is bounded by answer length, not
// input.
const checkChunk = 40

// Answer is one final decision, after the pair check.
type Answer struct {
	Key string
	// Decision is what the caller should judge with Verdict. Its ProductID is
	// nil where the model said "none", where the pair check refused the
	// proposal, or where the proposal could not be verified.
	Decision Decision
	// Offered says Decision.ProductID was in the window the model was shown.
	Offered bool
	// Proposed is what the proposal pass said, before the pair check.
	Proposed *int64
	// Checked says a pair check ran on this answer and returned.
	Checked bool
}

// Spec is one run.
type Spec struct {
	Profile Profile
	// Rows is the size of the file, for the adaptive ceilings. Questions are
	// usually far fewer, because identical rows collapse into one.
	Rows      int
	Questions []Question

	Feature        string
	OrganizationID int64
	UserID         int64

	// Screen, when set, reports whether a proposed product survives the
	// caller's mechanical identity check (productmatch.Index.IdentityConflict).
	// A proposal it refuses is not put to the pair check — Verdict refuses it
	// anyway, and the check would be money spent confirming a refusal.
	Screen func(key string, productID int64) bool

	// Describe, when set, projects a product the windows do not carry — the
	// engine's own settled answer, typically — so it can be put to the pair
	// check.
	Describe func(productID int64) (CatalogEntry, bool)

	// OnAnswer receives each final answer. It is called from several
	// goroutines; the caller synchronises.
	OnAnswer func(Answer)
	// OnDone receives each request's question keys once the request is
	// finished, answered or not, and finally the keys the budget never
	// reached. Progress is counted from it.
	OnDone func(keys []string)
}

// RunStats is what one run did.
type RunStats struct {
	Requests      int  `json:"requests"`
	CheckRequests int  `json:"check_requests"`
	Failed        int  `json:"failed"`
	Split         int  `json:"split"`
	Missing       int  `json:"missing"`
	Checked       int  `json:"checked"`
	CheckAccepted int  `json:"check_accepted"`
	CheckRejected int  `json:"check_rejected"`
	CheckUnsure   int  `json:"check_unsure"`
	Unasked       int  `json:"unasked"`
	CeilingHit    bool `json:"ceiling_hit"`

	// Diagnostic counters for proposal outcomes — they answer "why was
	// nothing applied?" when the model replies successfully but nothing
	// changes. Every item the model answered flows through exactly one of:
	//   Emitted (directly without check) + Checked (sent to pair check) = total answered
	ProposedNull int `json:"proposed_null"` // model said "none of these"
	ProposedLow  int `json:"proposed_low"`  // confidence below the floor
	Screened     int `json:"screened"`      // identity guard refused before pair check
	NotOffered   int `json:"not_offered"`   // product not in the window (hallucination)
	Emitted      int `json:"emitted"`       // sent directly to the caller without pair check
}

// Run asks every question it can inside the ceilings and hands back answers.
//
// It never fails. Every failure degrades to "no answer" for the rows it
// touched, which leaves those rows exactly as the deterministic engine did.
func Run(ctx context.Context, ai Enhancer, spec Spec) RunStats {
	var stats RunStats
	if ai == nil || len(spec.Questions) == 0 {
		return stats
	}

	rows := spec.Rows
	if rows < len(spec.Questions) {
		rows = len(spec.Questions)
	}
	c := Adaptive(spec.Profile, rows)
	wall := c.MaxWallClock
	if wall <= 0 || wall > hardWallClock {
		wall = hardWallClock
	}
	start := time.Now()
	runCtx, cancel := context.WithDeadline(ctx, start.Add(wall))
	defer cancel()
	proposeBy := start.Add(time.Duration(float64(wall) * proposeShare))

	requests, ceiling := Plan(spec.Questions, c)
	stats.CeilingHit = ceiling

	r := &runner{ai: ai, spec: spec, floor: c.MinApplyConfidence, stats: &stats}
	r.verifier, _ = ai.(Verifier)
	r.catalog = make(map[int64]CatalogEntry)
	for _, q := range spec.Questions {
		for _, e := range q.Window {
			if _, ok := r.catalog[e.ProductID]; !ok {
				r.catalog[e.ProductID] = e
			}
		}
	}

	asked := make(map[string]bool, len(spec.Questions))
	var (
		wg    sync.WaitGroup
		slots = make(chan struct{}, max(c.MaxConcurrent, 1))
	)
dispatch:
	for _, req := range requests {
		select {
		case slots <- struct{}{}:
		case <-runCtx.Done():
			r.mu.Lock()
			stats.CeilingHit = true
			r.mu.Unlock()
			break dispatch
		}
		if time.Now().After(proposeBy) {
			<-slots
			r.mu.Lock()
			stats.CeilingHit = true
			r.mu.Unlock()
			break
		}
		for _, k := range req.Keys {
			asked[k] = true
		}
		wg.Add(1)
		go func(req Request) {
			defer wg.Done()
			defer func() { <-slots }()
			defer r.done(req)
			defer func() {
				// One batch must never take the import down with it.
				if rec := recover(); rec != nil {
					r.mu.Lock()
					stats.Failed++
					r.mu.Unlock()
				}
			}()
			r.handle(runCtx, req)
		}(req)
	}
	wg.Wait()

	var unasked []string
	seen := make(map[string]bool)
	for _, q := range spec.Questions {
		if !asked[q.Key] && !seen[q.Key] {
			seen[q.Key] = true
			unasked = append(unasked, q.Key)
		}
	}
	stats.Unasked = len(unasked)
	if len(unasked) > 0 && spec.OnDone != nil {
		spec.OnDone(unasked)
	}
	stats.Requests = int(atomic.LoadInt64(&r.requests))
	stats.CheckRequests = int(atomic.LoadInt64(&r.checkRequests))
	return stats
}

type runner struct {
	ai       Enhancer
	verifier Verifier
	spec     Spec
	floor    float64
	catalog  map[int64]CatalogEntry

	requests      int64
	checkRequests int64

	mu    sync.Mutex
	stats *RunStats
}

func (r *runner) count(fn func(*RunStats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(r.stats)
}

func (r *runner) done(req Request) {
	if r.spec.OnDone == nil {
		return
	}
	keys := make([]string, 0, len(req.Keys))
	for _, ref := range sortedRefs(req.Keys) {
		keys = append(keys, req.Keys[ref])
	}
	r.spec.OnDone(keys)
}
