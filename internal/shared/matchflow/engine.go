package matchflow

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// splitMin is the smallest batch that is split rather than given up on.
const splitMin = 6

// maxDepth bounds re-asking: a failed batch is halved at most this many times.
const maxDepth = 3

// maxAttemptsPerQuestion bounds attempts per question across proposal, retry, split, and verify.
const maxAttemptsPerQuestion = 3

// checkChunk is how many pairs one verification request carries.
const checkChunk = 40

// Answer is one final decision, after pair check if verified.
type Answer struct {
	Key string
	// Decision is what the caller should judge with Verdict.
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
	Profile   Profile
	Rows      int
	Questions []Question

	Feature        string
	OrganizationID int64
	UserID         int64

	Budget            *Budget
	MaxCostPerAttempt money.Amount
	VerifyPass        bool
	EnqueueTime       time.Time
	Deadline          time.Duration
	OnPublishTerminal func(RunStats)

	// Screen, when set, reports whether a proposed product survives identity checks.
	Screen func(key string, productID int64) bool

	// Describe projects a product the windows do not carry so it can be verified.
	Describe func(productID int64) (CatalogEntry, bool)

	// OnAnswer receives each final answer progressively.
	OnAnswer func(Answer)

	// OnDone receives each request's question keys once finished.
	OnDone func(keys []string)
}

// RunStats captures comprehensive run metrics.
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

	// Diagnostic counters for proposal outcomes.
	ProposedNull int `json:"proposed_null"` // model said "none of these"
	ProposedLow  int `json:"proposed_low"`  // confidence below the floor
	Screened     int `json:"screened"`      // identity guard refused before pair check
	NotOffered   int `json:"not_offered"`   // product not in the window (hallucination)
	Emitted      int `json:"emitted"`       // sent directly to caller

	// Outcome categorization.
	Unattempted int `json:"unattempted"`
	Abstained   int `json:"abstained"`
	Rejected    int `json:"rejected"`
	AcceptedNew int `json:"accepted_new"`
}

// Run executes the AI matching stage within deadlines and budgets.
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

	runDeadline := spec.Deadline
	if runDeadline <= 0 {
		runDeadline = DeadlineClass(len(spec.Questions))
	}
	origin := spec.EnqueueTime
	if origin.IsZero() {
		origin = time.Now()
	}
	elapsed := time.Since(origin)
	if elapsed < 0 {
		elapsed = 0
	}
	wall := runDeadline - elapsed
	if wall <= 0 {
		stats.CeilingHit = true
		stats.Unasked = len(spec.Questions)
		stats.Unattempted = len(spec.Questions)
		return stats
	}

	runCtx, cancel := context.WithDeadline(ctx, origin.Add(runDeadline))
	defer cancel()
	proposeBy := origin.Add(time.Duration(float64(runDeadline) * DispatchShare))

	requests, ceiling := Plan(spec.Questions, c)
	stats.CeilingHit = ceiling

	r := &runner{
		ai:               ai,
		spec:             spec,
		floor:            c.MinApplyConfidence,
		stats:            &stats,
		origin:           origin,
		runDeadline:      runDeadline,
		questionAttempts: make(map[string]int),
	}
	if spec.VerifyPass {
		r.verifier, _ = ai.(Verifier)
	}
	r.catalog = make(map[int64]CatalogEntry)
	for _, q := range spec.Questions {
		for _, e := range q.Window {
			if _, ok := r.catalog[e.ProductID]; !ok {
				r.catalog[e.ProductID] = e
			}
		}
	}

	var terminalPublished int32
	publishTerminal := func() {
		if atomic.CompareAndSwapInt32(&terminalPublished, 0, 1) {
			if spec.OnPublishTerminal != nil {
				r.mu.Lock()
				snap := *r.stats
				r.mu.Unlock()
				spec.OnPublishTerminal(snap)
			}
		}
	}
	terminalTimer := time.AfterFunc(runDeadline+TerminalPublishSlack-elapsed, publishTerminal)
	defer terminalTimer.Stop()

	asked := make(map[string]bool, len(spec.Questions))
	var (
		wg    sync.WaitGroup
		slots = make(chan struct{}, max(c.MaxConcurrent, 1))
	)
dispatch:
	for _, req := range requests {
		if time.Now().After(proposeBy) {
			r.mu.Lock()
			stats.CeilingHit = true
			r.mu.Unlock()
			break dispatch
		}
		if spec.Budget != nil {
			if !spec.Budget.Reserve(spec.MaxCostPerAttempt) {
				r.mu.Lock()
				stats.CeilingHit = true
				r.mu.Unlock()
				break dispatch
			}
		}
		select {
		case slots <- struct{}{}:
		case <-runCtx.Done():
			if spec.Budget != nil {
				spec.Budget.Settle(spec.MaxCostPerAttempt, money.Zero, true)
			}
			r.mu.Lock()
			stats.CeilingHit = true
			r.mu.Unlock()
			break dispatch
		}
		for _, k := range req.Keys {
			asked[k] = true
		}
		wg.Add(1)
		go func(req Request) {
			defer wg.Done()
			defer func() { <-slots }()
			defer func() {
				if spec.Budget != nil {
					spec.Budget.Settle(spec.MaxCostPerAttempt, spec.MaxCostPerAttempt, true)
				}
			}()
			defer r.done(req)
			defer func() {
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
	stats.Unattempted = len(unasked)
	if len(unasked) > 0 && spec.OnDone != nil {
		spec.OnDone(unasked)
	}
	stats.Requests = int(atomic.LoadInt64(&r.requests))
	stats.CheckRequests = int(atomic.LoadInt64(&r.checkRequests))

	publishTerminal()
	return stats
}

type runner struct {
	ai          Enhancer
	verifier    Verifier
	spec        Spec
	floor       float64
	catalog     map[int64]CatalogEntry
	origin      time.Time
	runDeadline time.Duration

	requests      int64
	checkRequests int64

	mu               sync.Mutex
	stats            *RunStats
	questionAttempts map[string]int
}

func (r *runner) count(fn func(*RunStats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(r.stats)
}

func (r *runner) recordAttempt(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.questionAttempts[key] >= maxAttemptsPerQuestion {
		return false
	}
	r.questionAttempts[key]++
	return true
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
