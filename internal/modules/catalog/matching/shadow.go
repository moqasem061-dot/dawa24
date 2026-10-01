package matching

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ShadowWorker executes asynchronous shadow matching policy evaluation (US13 T151).
// Writes strictly to catalog.match_shadow_outcomes without altering operational review paths.
type ShadowWorker struct {
	river.WorkerDefaults[queue.MatchShadowArgs]
	svc matchingServiceInternal
	log *slog.Logger
}

type matchingServiceInternal interface {
	RunShadow(ctx context.Context, runID int64, orgID int64, shadowVersion string) error
}

// NewShadowWorker constructs a ShadowWorker for River.
func NewShadowWorker(svc Service, log *slog.Logger) *ShadowWorker {
	var internal matchingServiceInternal
	if s, ok := svc.(matchingServiceInternal); ok {
		internal = s
	}
	return &ShadowWorker{
		svc: internal,
		log: log,
	}
}

// Work implements the River worker interface for MatchShadowArgs.
func (w *ShadowWorker) Work(ctx context.Context, job *river.Job[queue.MatchShadowArgs]) error {
	if w.svc == nil {
		if w.log != nil {
			w.log.WarnContext(ctx, "shadow worker: service does not implement matchingServiceInternal")
		}
		return nil
	}
	return w.svc.RunShadow(ctx, job.Args.RunID, job.Args.OrganizationID, job.Args.ShadowVersion)
}

// RunShadow executes the shadow policy for a run and records outcomes in catalog.match_shadow_outcomes.
func (s *serviceImpl) RunShadow(ctx context.Context, runID int64, orgID int64, shadowVersion string) error {
	if shadowVersion == "" || s.repo == nil {
		return nil
	}

	// 1. Fetch shadow policy
	shadowPol, err := s.repo.GetPolicy(ctx, shadowVersion)
	if err != nil {
		return fmt.Errorf("run shadow: fetch policy %s: %w", shadowVersion, err)
	}

	// 2. Fetch run details
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("run shadow: fetch run %d: %w", runID, err)
	}

	// 3. Load catalog index
	idx, err := s.indexCache.Get(ctx, "master", run.CatalogRevision)
	if err != nil || idx == nil {
		return fmt.Errorf("run shadow: index unavailable for revision %d", run.CatalogRevision)
	}
	if aliases, aErr := s.repo.LoadAliases(ctx, orgID); aErr == nil && len(aliases) > 0 {
		idx = idx.WithAliases(aliases)
	}

	// 4. Retrieve rows: check in-memory cache first, otherwise load from repository
	var rowsToDecide []shadowTargetRow

	s.shadowRowsMu.Lock()
	cachedRows, hasCached := s.shadowRows[runID]
	delete(s.shadowRows, runID)
	s.shadowRowsMu.Unlock()

	if hasCached && len(cachedRows) > 0 {
		for i, r := range cachedRows {
			rowNum := r.Number
			if rowNum <= 0 {
				rowNum = i + 1
			}
			rowRef := fmt.Sprintf("row_%d", rowNum)
			rowsToDecide = append(rowsToDecide, shadowTargetRow{
				RowRef: rowRef,
				Row:    r,
			})
		}
	} else {
		enhanceRows, lErr := s.repo.ListEnhanceRows(ctx, runID, run.Workflow)
		if lErr != nil {
			return fmt.Errorf("run shadow: list enhance rows for run %d: %w", runID, lErr)
		}
		for i, er := range enhanceRows {
			rowNum := er.RowRef.RowNumber
			if rowNum <= 0 {
				rowNum = i + 1
			}
			rowRef := fmt.Sprintf("row_%d", rowNum)
			if er.RowRef.ID > 0 {
				rowRef = strconv.FormatInt(er.RowRef.ID, 10)
			}
			rowsToDecide = append(rowsToDecide, shadowTargetRow{
				RowRef: rowRef,
				Row:    er.Row,
			})
		}
	}

	if len(rowsToDecide) == 0 {
		return nil
	}

	// 5. Deduplicate decisions by QuestionSignature (In-run dedup)
	sigMap := make(map[string]matchflow.RowDecision)
	outcomes := make([]ShadowOutcome, 0, len(rowsToDecide))

	for _, target := range rowsToDecide {
		rowFacts := productmatch.ExtractRowFacts(&target.Row, nil)
		sig := productmatch.QuestionSignature(rowFacts, target.Row.Name, productmatch.NormalizerVersion)

		dec, found := sigMap[sig]
		if !found {
			dec = idx.Decide(&target.Row, shadowPol)
			dec.Revision = run.SourceRevision
			dec.Versions.Policy = shadowVersion
			dec.Versions.CatalogRevision = run.CatalogRevision
			dec.Versions.Normalizer = productmatch.NormalizerVersion
			sigMap[sig] = dec
		}

		outcomes = append(outcomes, ShadowOutcome{
			RunID:               runID,
			OrganizationID:      orgID,
			RowRef:              target.RowRef,
			ShadowPolicyVersion: shadowVersion,
			Decision:            dec,
			CreatedAt:           time.Now().UTC(),
		})
	}

	// 6. Record shadow outcomes strictly in catalog.match_shadow_outcomes
	if err := s.repo.RecordShadowOutcomes(ctx, outcomes); err != nil {
		return fmt.Errorf("run shadow: persist outcomes: %w", err)
	}

	return nil
}

type shadowTargetRow struct {
	RowRef string
	Row    productmatch.Row
}
