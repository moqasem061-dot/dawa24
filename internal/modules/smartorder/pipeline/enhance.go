package pipeline

// The AI enhancement stage.
//
// It runs after the deterministic engine has finished and seen the whole file,
// and it exists to answer the lines that engine could not: the ones it left
// unmatched (غير مطابق) and the ones it matched only tentatively, below the
// cutoff (مطلوب للمراجعة). Nothing it produces overwrites a confident
// deterministic result, and everything it produces is re-checked before it is
// written.
//
// Why the previous shape did not help, and what changed:
//
//   - It offered the model the five candidates the scorer had picked — the same
//     scorer that had just failed on that row. If the correct product was not in
//     those five, no amount of intelligence could find it, and when the scorer
//     found nothing at all the line was skipped entirely. On live data that was
//     most of the residue. Retrieval is now a separate, recall-tuned pass
//     (productmatch.Recall) that unions token, trigram and molecule strategies,
//     and lines with no shortlist are exactly the ones that need it most.
//   - It sent twenty-five rows per request with the candidates repeated inside
//     each one, so a run of five hundred residual lines was twenty requests, and
//     a ninety-second wall clock cut that off at seven. Candidates are now
//     de-duplicated into ONE catalogue window shared by every item in the
//     request, which is what makes a whole file fit in a single call.
//   - It scored a decision only against that row's own shortlist. The model may
//     now answer with any product in the window, which repairs the commonest
//     retrieval failure of all: the right product was retrieved — for the line
//     above.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// PromptVersion is the version of the question being asked, and it is part of
// the decision-cache key.
//
// It is the shared one. It used to be declared here, and it had drifted from
// the copy the other importer declared — so the two filed answers to the same
// question under different keys and neither ever read the other's. A build gate
// now fails if the literal is restated outside internal/shared/matchflow.
const PromptVersion = matchflow.PromptVersion

// ceilings are what one run may spend, from the shared table.
//
// They used to be a const block here and another in the other importer, with
// every number documented as measured and no two of them agreeing.
var ceilings = matchflow.For(matchflow.ProfileOrder)

// Enhancer resolves a batch of review lines against a catalogue window.
//
// Declared here, in the pipeline's own vocabulary, so this package never imports
// the gateway and every test in it runs without one (AGENTS.md R2, R5).
type Enhancer interface {
	Enhance(ctx context.Context, batch EnhanceBatch) ([]EnhanceOutcome, error)
}

// EnhanceBatch is one request: a de-duplicated catalogue window and the lines to
// resolve against it.
type EnhanceBatch = matchflow.Batch

// WindowProduct is one catalogue product offered to the model.
type WindowProduct = matchflow.CatalogEntry

// ReviewLine is one line needing a second opinion, as the model sees it.
type ReviewLine = matchflow.Item

// EnhanceOutcome is one decision, keyed by the request-local ref.
type EnhanceOutcome = matchflow.Decision

// Review is one line carried into the AI stage with its retrieval attached.
type Review struct {
	Line       *smartorder.Line
	Row        *productmatch.Row
	Candidates []productmatch.MatchCandidate
	// Settled says the deterministic engine already applied this line's match,
	// so the question is verification rather than resolution. See
	// matchflow.Verdict for what the difference costs an answer.
	Settled bool
	// Ambiguous says the scorer found two candidates and nothing in the row to
	// choose between them. It is the highest-priority question in any file.
	Ambiguous bool
}

// EnhancementStats is what the run records about this stage.
//
// Improved is the number that answers the question the buyer actually has —
// "did the AI do anything for me?" — and it counts lines whose matched product
// changed, not lines the model replied about.
type EnhancementStats struct {
	Reviewed  int
	CacheHits int
	Requests  int
	Improved  int
	Confirmed int
	Abstained int
	Rejected  int
	// Disputed counts lines the engine had applied and the model would not
	// confirm. They are the reason the verification pass exists: two
	// independent methods disagreeing about one row is the strongest signal a
	// file produces that the row is wrong, and it is invisible without asking.
	Disputed int
	// Verified counts the settled lines that were put to the model at all,
	// which is what makes Disputed a rate rather than a number.
	Verified int
	// RefusedBy counts refusals by conflict kind — strength, modifier, form,
	// evidence. It is the number that says whether the guards are protecting the
	// buyer or fighting the model, and it is the first thing to look at when
	// this stage stops helping.
	RefusedBy  map[string]int
	CeilingHit bool
}

// Enhancement runs the AI stage under its ceilings.
type Enhancement struct {
	repo  smartorder.Repository
	ai    Enhancer
	index *productmatch.Index
	log   *slog.Logger
	now   func() time.Time

	requests int64
	settled  int64

	// OnProgress, when set, receives the running count of lines settled. It is
	// the only thing that moves the bar during the one stage that waits on a
	// network, and it is called from several goroutines.
	OnProgress func(done int)

	mu        sync.Mutex
	persistMu sync.Mutex
	Stats     EnhancementStats
}

// NewEnhancement constructs the AI stage. A nil logger is allowed; refusals are
// then simply not recorded.
func NewEnhancement(repo smartorder.Repository, ai Enhancer, index *productmatch.Index,
	log *slog.Logger) *Enhancement {
	return &Enhancement{
		repo:  repo,
		ai:    ai,
		index: index,
		log:   log,
		now:   time.Now,
		Stats: EnhancementStats{RefusedBy: map[string]int{}},
	}
}

// Run improves what it can and leaves the rest as the deterministic engine found
// it.
//
// It never returns an error that fails the run. Every failure path — Gateway
// down, budget exhausted, malformed response — degrades to the deterministic
// outcome, because a pharmacy must be able to order when the AI is unavailable.
func (e *Enhancement) Run(ctx context.Context, reviews []Review) {
	if e.ai == nil || e.index == nil || len(reviews) == 0 {
		return
	}

	deadline := e.now().Add(ceilings.MaxWallClock)
	// The wall clock is enforced on the calls themselves, not merely checked
	// between them: a check in the dispatch loop passes for every batch and
	// then lets slow requests run past the limit unbounded.
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for _, r := range reviews {
		if r.Settled {
			e.Stats.Verified++
		}
	}

	// The cache answers first, so a remembered question never enters a request.
	groups := e.applyCache(ctx, byKey(reviews))
	if len(groups) == 0 {
		return
	}

	questions := make([]matchflow.Question, 0, len(groups))
	for key, group := range groups {
		for _, q := range e.questions(group[:1]) {
			q.Key = key
			questions = append(questions, q)
		}
	}
	adaptiveCeilings := matchflow.Adaptive(matchflow.ProfileOrder, len(reviews))
	requests, ceilingHit := matchflow.Plan(questions, adaptiveCeilings)
	if ceilingHit {
		e.Stats.CeilingHit = true
	}

	var (
		wg    sync.WaitGroup
		slots = make(chan struct{}, adaptiveCeilings.MaxConcurrent)
	)

	for _, req := range requests {
		wg.Add(1)
		slots <- struct{}{}
		go func(r matchflow.Request) {
			defer wg.Done()
			defer func() { <-slots }()
			lines := linesIn(r, groups)
			// A panic inside one batch must not take the buyer's whole import
			// down with it.
			defer func() {
				if rec := recover(); rec != nil {
					e.count(func(s *EnhancementStats) { s.Rejected += lines })
				}
			}()
			// The batch is accounted whatever happens to it: a failed batch
			// still leaves its lines with their deterministic outcome, and a bar
			// that stalls on failure reads as a hung run.
			defer e.report(lines)

			atomic.AddInt64(&e.requests, 1)
			r.Batch.Feature = matchflow.FeatureSmartOrder
			outcomes, err := e.callWithRetry(runCtx, r.Batch)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					e.count(func(s *EnhancementStats) { s.CeilingHit = true })
				}
				return
			}

			saved := e.apply(r, groups, outcomes)
			e.flush(runCtx, saved)
		}(req)
	}

	wg.Wait()

	e.Stats.Requests = int(atomic.LoadInt64(&e.requests))
}

// linesIn counts the buyer's rows one request settles.
//
// It differs from the item count whenever duplicates were collapsed, and it is
// what the progress bar counts — a bar measured in questions rather than rows
// would stop short of the total on any file that repeats a product.
func linesIn(r matchflow.Request, groups map[string][]Review) int {
	n := 0
	for _, key := range r.Keys {
		n += len(groups[key])
	}
	return n
}

// callWithRetry executes an enhancement batch with a per-request timeout (60s)
// and retries once on transient errors, protecting against hung connections
// and occasional gateway 503/499 context cancellations.
func (e *Enhancement) callWithRetry(ctx context.Context, batch EnhanceBatch) ([]EnhanceOutcome, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		outcomes, err := e.ai.Enhance(reqCtx, batch)
		cancel()
		if err == nil {
			return outcomes, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, lastErr
}

// report advances the progress callback by one batch.
func (e *Enhancement) report(n int) {
	if e.OnProgress == nil {
		return
	}
	e.OnProgress(int(atomic.AddInt64(&e.settled, int64(n))))
}

// count mutates the stats under the same lock apply uses.
func (e *Enhancement) count(fn func(*EnhancementStats)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(&e.Stats)
}
