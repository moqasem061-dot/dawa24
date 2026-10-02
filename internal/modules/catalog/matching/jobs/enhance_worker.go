package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// EnhanceWorker processes asynchronous AI matching enhancement jobs (US9 T125).
type EnhanceWorker struct {
	river.WorkerDefaults[queue.MatchEnhanceArgs]
	svc        matching.Service
	repo       matching.Repository
	indexCache *matching.IndexCache
	enhancer   matchflow.Enhancer
	scheduler  *matching.FairScheduler
	log        *slog.Logger
}

// NewEnhanceWorker constructs an EnhanceWorker.
func NewEnhanceWorker(
	svc matching.Service,
	repo matching.Repository,
	indexCache *matching.IndexCache,
	enhancer matchflow.Enhancer,
	scheduler *matching.FairScheduler,
	log *slog.Logger,
) *EnhanceWorker {
	if scheduler == nil {
		scheduler = matching.GlobalFairScheduler
	}
	return &EnhanceWorker{
		svc:        svc,
		repo:       repo,
		indexCache: indexCache,
		enhancer:   enhancer,
		scheduler:  scheduler,
		log:        log,
	}
}

// Work executes the AI matching run for a submitted job.
func (w *EnhanceWorker) Work(ctx context.Context, job *river.Job[queue.MatchEnhanceArgs]) error {
	runID := job.Args.RunID
	orgID := job.Args.OrganizationID

	run, err := w.repo.GetRun(ctx, runID)
	if err != nil {
		w.log.WarnContext(ctx, "enhance worker: run not found", "run_id", runID, "error", err)
		return nil
	}
	if run.State == "cancelled" {
		w.log.InfoContext(ctx, "enhance worker: run was cancelled", "run_id", runID)
		return nil
	}

	stopHB := matching.StartHeartbeat(ctx, w.repo, run.ID, matching.DefaultHeartbeatInterval)
	defer stopHB()

	applier := w.svc.GetApplier(matching.Workflow(run.Workflow))
	if applier == nil {
		_ = w.repo.SetState(ctx, runID, "failed", true)
		return fmt.Errorf("enhance worker: no applier registered for workflow %s", run.Workflow)
	}

	rows, err := w.repo.ListEnhanceRows(ctx, run.ID, run.Workflow)
	if err != nil {
		_ = w.repo.SetState(ctx, runID, "failed", true)
		return fmt.Errorf("enhance worker: load rows for run %d: %w", runID, err)
	}
	if len(rows) == 0 {
		return w.finishRun(ctx, runID, map[string]any{"ran": false, "reason": "empty_rows"})
	}

	// Filter eligible rows (T122)
	var eligibleRows []matching.EnhanceRow
	for _, r := range rows {
		if ok, _ := matchflow.Eligible(r.Decision, false, false); ok {
			eligibleRows = append(eligibleRows, r)
		}
	}
	if len(eligibleRows) == 0 {
		return w.finishRun(ctx, runID, map[string]any{"ran": false, "reason": "no_eligible_rows"})
	}

	var idx *productmatch.Index
	if w.indexCache != nil {
		catRev, cErr := w.repo.CurrentRevision(ctx, "master")
		if cErr != nil {
			catRev = 1
		}
		idx, _ = w.indexCache.Get(ctx, "master", catRev)
	}

	// Group rows by signature into questions
	qMap := make(map[string][]matching.EnhanceRow)
	var questions []matchflow.Question
	for _, er := range eligibleRows {
		rowFacts := productmatch.ExtractRowFacts(&er.Row, nil)
		sig := productmatch.QuestionSignature(rowFacts, er.Row.Name, productmatch.NormalizerVersion)

		if _, exists := qMap[sig]; !exists {
			var window []matchflow.CatalogEntry
			var options []int64
			for _, c := range er.Decision.Candidates {
				options = append(options, c.ProductID)
				if idx != nil {
					if p, found := idx.Lookup(c.ProductID); found {
						window = append(window, matchflow.CatalogEntry{
							ProductID:     p.ID,
							NameAR:        p.NameAR,
							NameEN:        p.NameEN,
							DosageForm:    p.DosageForm,
							Concentration: p.Concentration,
							Manufacturer:  p.Manufacturer,
						})
					}
				}
			}
			q := matchflow.Question{
				Key: sig,
				Item: matchflow.Item{
					Text:    er.Row.Name,
					Options: options,
				},
				Window: window,
				Risk:   1.0,
			}
			questions = append(questions, q)
		}
		qMap[sig] = append(qMap[sig], er)
	}

	var attemptCounter int32
	var appliedCount int32

	// Wrap enhancer with fair-share scheduler and attempt recording (T119, T125, T126)
	recordingEnhancer := matchflow.EnhancerFunc(func(eCtx context.Context, batch matchflow.Batch) ([]matchflow.Decision, error) {
		attNo := int(atomic.AddInt32(&attemptCounter, 1))
		release, sErr := w.scheduler.AcquireSlot(eCtx, orgID)
		if sErr != nil {
			return nil, sErr
		}
		defer release()

		batchBytes, _ := json.Marshal(batch)
		sum := sha256.Sum256(batchBytes)
		reqSHA := hex.EncodeToString(sum[:])

		start := time.Now().UTC()
		decisions, eErr := w.enhancer.Enhance(eCtx, batch)
		end := time.Now().UTC()

		outcome := "ok"
		if eErr != nil {
			outcome = "error"
		}
		ansJSON, _ := json.Marshal(decisions)

		_ = w.repo.RecordAIAttempt(eCtx, &matching.AIAttemptRecord{
			RunID:          run.ID,
			OrganizationID: orgID,
			AttemptNo:      attNo,
			Kind:           "proposal",
			QuestionCount:  len(batch.Items),
			StartedAt:      start,
			DeadlineAt:     start.Add(15 * time.Second),
			EndedAt:        &end,
			Outcome:        outcome,
			RequestSHA256:  reqSHA,
			Answers:        ansJSON,
		})

		return decisions, eErr
	})

	spec := matchflow.Spec{
		Questions:      questions,
		Rows:           len(eligibleRows),
		OrganizationID: orgID,
		EnqueueTime:    job.Args.EnqueuedAt,
		Describe: func(productID int64) (matchflow.CatalogEntry, bool) {
			if idx == nil {
				return matchflow.CatalogEntry{}, false
			}
			p, found := idx.Lookup(productID)
			if !found {
				return matchflow.CatalogEntry{}, false
			}
			return matchflow.CatalogEntry{
				ProductID:     p.ID,
				NameAR:        p.NameAR,
				NameEN:        p.NameEN,
				DosageForm:    p.DosageForm,
				Concentration: p.Concentration,
				Manufacturer:  p.Manufacturer,
			}, true
		},
		// The identity guard every live AI path uses: a model may not accept a
		// product the deterministic engine's own conflict model refuses.
		Screen: func(key string, productID int64) bool {
			rows := qMap[key]
			return idx != nil && len(rows) > 0 && idx.IdentityConflict(&rows[0].Row, productID).None()
		},
		OnAnswer: func(ans matchflow.Answer) {
			if ans.Decision.ProductID == nil || idx == nil {
				return
			}
			targetID := *ans.Decision.ProductID
			affectedRows := qMap[ans.Key]
			if len(affectedRows) == 0 {
				return
			}
			j := matchflow.Judgement{
				Offered:    ans.Offered,
				Confidence: ans.Decision.Confidence,
				Floor:      enhanceCeilings.MinApplyConfidence,
				Verified:   ans.Checked,
				Conflicts:  !idx.IdentityConflict(&affectedRows[0].Row, targetID).None(),
			}
			if matchflow.Verdict(j, ans.Decision.ProductID) != matchflow.OutcomeApply {
				return
			}
			for _, r := range affectedRows {
				newDec := r.Decision
				newDec.State = matchflow.StateAccepted
				newDec.Basis = matchflow.BasisAI
				newDec.ProductID = &targetID
				newDec.AI = matchflow.AIDecision{
					State:    matchflow.AIStateCompleted,
					Proposal: &targetID,
					Outcome:  "accepted",
				}
				applied, aErr := applier.ApplyAI(ctx, r.RowRef, r.Decision.Revision, run.ID, newDec)
				if aErr == nil && applied {
					atomic.AddInt32(&appliedCount, 1)
				}
			}
		},
	}

	stats := matchflow.Run(ctx, recordingEnhancer, spec)

	aiCounters := map[string]any{
		"ran":          true,
		"attempts":     int(atomic.LoadInt32(&attemptCounter)),
		"applied":      int(atomic.LoadInt32(&appliedCount)),
		"unattempted":  stats.Unattempted,
		"abstained":    stats.Abstained,
		"rejected":     stats.Rejected,
		"accepted_new": stats.AcceptedNew,
		"ceiling_hit":  stats.CeilingHit,
	}

	return w.finishRun(ctx, runID, aiCounters)
}

// enhanceCeilings are the catalogue profile's limits, the floor among them.
var enhanceCeilings = matchflow.For(matchflow.ProfileCatalog)

func (w *EnhanceWorker) finishRun(ctx context.Context, runID int64, aiCounters map[string]any) error {
	_ = w.repo.UpdateAIRunCounters(ctx, runID, aiCounters)
	return w.repo.SetState(ctx, runID, "reviewable", true)
}
