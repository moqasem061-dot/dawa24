package matching

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Service defines the orchestrating interface for matching operations.
type Service interface {
	Prepare(ctx context.Context, req PrepareRequest) (PrepareResult, error)
	GetRun(ctx context.Context, runID int64) (*MatchRun, error)
	ListRunRows(ctx context.Context, sc Scope, runID int64, limit int) ([]DiagnosticRowData, error)
	Diagnose(ctx context.Context, sc Scope, runID int64, row productmatch.Row, dec matchflow.RowDecision) (DiagnosticRow, error)
	MarkReviewable(ctx context.Context, runID int64) error
	Rematch(ctx context.Context, s Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error)
	Search(ctx context.Context, s Scope, q string, row productmatch.Row, limit int) ([]matchflow.CandidateView, error)
	Review(ctx context.Context, s Scope, a ReviewAction) (matchflow.RowDecision, error)
	PreviewBulk(ctx context.Context, s Scope, a ReviewAction, rows []RowRef) (BulkPreview, error)
	StartEnhancement(ctx context.Context, runID int64, rows []EnhanceRow) (EnhanceHandle, error)
	StopEnhancement(ctx context.Context, runID int64) error
	Revalidate(ctx context.Context, s Scope, ref RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
	Remember(ctx context.Context, s Scope, k KnowledgeWrite) error
	ListKnowledge(ctx context.Context, s Scope, f KnowledgeFilter) (Page[Knowledge], error)
	RevokeKnowledge(ctx context.Context, s Scope, id int64) error
	PromoteKnowledge(ctx context.Context, s Scope, id int64) error
}

type serviceImpl struct {
	repo       Repository
	indexCache *IndexCache
	loader     *Loader
	decideFn   func(idx *productmatch.Index, row *productmatch.Row, pol productmatch.Policy) matchflow.RowDecision
}

// NewService constructs a matching Service.
func NewService(repo Repository, indexCache *IndexCache, loader *Loader) Service {
	return &serviceImpl{
		repo:       repo,
		indexCache: indexCache,
		loader:     loader,
		decideFn: func(idx *productmatch.Index, row *productmatch.Row, pol productmatch.Policy) matchflow.RowDecision {
			return idx.Decide(row, pol)
		},
	}
}

// Prepare executes deterministic matching on source rows, deduplicating by QuestionSignature.
func (s *serviceImpl) Prepare(ctx context.Context, req PrepareRequest) (PrepareResult, error) {
	n := len(req.Rows)
	var summary matchflow.RunCounts
	decisions := make([]matchflow.RowDecision, n)

	// 1. Resolve policy
	pol, err := s.repo.ActivePolicy(ctx, string(req.Scope.Workflow), req.Scope.EffectiveOrg)
	if err != nil || pol.Version == "" {
		pol = productmatch.DefaultPolicy()
	}
	if req.Required != productmatch.GranularityNone {
		pol.Required = req.Required
	}
	if req.Identifiers != nil {
		pol.Identifiers = *req.Identifiers
	}

	// 2. Get revision & index (cache)
	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		// Catalogue load error -> every row is unavailable with catalog_unavailable
		for i := range req.Rows {
			dec := matchflow.RowDecision{
				V:        1,
				State:    matchflow.StateUnavailable,
				Basis:    matchflow.BasisSimilarity,
				Reasons:  []matchflow.Reason{{Code: string(productmatch.ReasonCatalogUnavailable)}},
				Revision: req.SourceRev,
				Versions: matchflow.Versions{
					Policy:          pol.Version,
					Normalizer:      productmatch.NormalizerVersion,
					CatalogRevision: catRev,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateOff},
			}
			decisions[i] = dec
			summary.AddState(matchflow.StateUnavailable)
		}

		run := &MatchRun{
			OrganizationID:    req.Scope.EffectiveOrg,
			Workflow:          string(req.Scope.Workflow),
			EntryPoint:        req.Scope.EntryPoint,
			ConsumerRef:       req.ConsumerRef,
			SourceRevision:    req.SourceRev,
			PolicyVersion:     pol.Version,
			CatalogRevision:   catRev,
			NormalizerVersion: productmatch.NormalizerVersion,
			State:             "failed",
			Counts:            summary,
		}
		_ = s.repo.CreateRun(ctx, run)
		return PrepareResult{
			RunID:     run.ID,
			Decisions: decisions,
			Summary:   summary,
		}, nil
	}

	if s.repo != nil {
		if aliases, aErr := s.repo.LoadAliases(ctx, req.Scope.EffectiveOrg); aErr == nil && len(aliases) > 0 {
			idx = idx.WithAliases(aliases)
		}
	}

	// 3. Group rows by QuestionSignature (In-run dedup, FR-034)
	type sigGroup struct {
		repRow productmatch.Row
		rowIdx []int
	}
	sigMap := make(map[string]*sigGroup)
	var sigOrder []string

	for i := range req.Rows {
		row := req.Rows[i]
		rowFacts := productmatch.ExtractRowFacts(&row, nil)
		sig := productmatch.QuestionSignature(rowFacts, row.Name, productmatch.NormalizerVersion)

		grp, ok := sigMap[sig]
		if !ok {
			grp = &sigGroup{
				repRow: row,
				rowIdx: make([]int, 0, 4),
			}
			sigMap[sig] = grp
			sigOrder = append(sigOrder, sig)
		}
		grp.rowIdx = append(grp.rowIdx, i)
	}

	summary.DistinctQuestions = len(sigOrder)

	// 4. Create match_runs row in 'preparing' state and start heartbeat ticker (US12 T102)
	run := &MatchRun{
		OrganizationID:    req.Scope.EffectiveOrg,
		Workflow:          string(req.Scope.Workflow),
		EntryPoint:        req.Scope.EntryPoint,
		ConsumerRef:       req.ConsumerRef,
		SourceRevision:    req.SourceRev,
		PolicyVersion:     pol.Version,
		CatalogRevision:   catRev,
		NormalizerVersion: productmatch.NormalizerVersion,
		State:             "preparing",
	}
	if s.repo != nil {
		_ = s.repo.CreateRun(ctx, run)
	}
	if run.ID > 0 {
		stopHB := StartHeartbeat(ctx, s.repo, run.ID, DefaultHeartbeatInterval)
		defer stopHB()
	}

	// 5. Look up knowledge first; fallback to Index.Decide once per signature, with panic recovery, fan out to rows
	kr := NewKnowledgeResolver(s.repo)
	for _, sig := range sigOrder {
		grp := sigMap[sig]
		repRow := grp.repRow

		var dec matchflow.RowDecision
		if kDec, ok, kErr := kr.Resolve(ctx, req.Scope, &repRow, sig, idx, pol, nil); kErr == nil && ok && kDec != nil {
			dec = *kDec
			dec.Revision = req.SourceRev
			if dec.State == matchflow.StateAccepted {
				summary.KnowledgeReuse++
			}
		} else {
			dec = s.decideWithRecover(idx, &repRow, pol, req.SourceRev, catRev)
		}

		for _, rowIdx := range grp.rowIdx {
			rowDec := dec
			decisions[rowIdx] = rowDec
			summary.AddState(rowDec.State)
		}
	}

	// 6. Update counts on match_runs
	run.Counts = summary
	if s.repo != nil && run.ID > 0 {
		_ = s.repo.UpdateCounts(ctx, run.ID, summary)
	}

	return PrepareResult{
		RunID:     run.ID,
		Decisions: decisions,
		Summary:   summary,
	}, nil
}

func (s *serviceImpl) decideWithRecover(
	idx *productmatch.Index,
	row *productmatch.Row,
	pol productmatch.Policy,
	sourceRev int,
	catRev int64,
) (dec matchflow.RowDecision) {
	defer func() {
		if r := recover(); r != nil {
			dec = matchflow.RowDecision{
				V:        1,
				State:    matchflow.StateInvalidInput,
				Basis:    matchflow.BasisSimilarity,
				Reasons:  []matchflow.Reason{{Code: string(productmatch.ReasonNumericUninterpretable)}},
				Revision: sourceRev,
				Versions: matchflow.Versions{
					Policy:          pol.Version,
					Normalizer:      productmatch.NormalizerVersion,
					CatalogRevision: catRev,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateOff},
			}
		}
	}()

	decFn := s.decideFn
	if decFn == nil {
		decFn = func(i *productmatch.Index, r *productmatch.Row, p productmatch.Policy) matchflow.RowDecision {
			return i.Decide(r, p)
		}
	}
	dec = decFn(idx, row, pol)
	dec.Revision = sourceRev
	return dec
}

// GetRun retrieves a match run by ID.
func (s *serviceImpl) GetRun(ctx context.Context, runID int64) (*MatchRun, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	return s.repo.GetRun(ctx, runID)
}

// ListRunRows lists rows associated with a match run for diagnostics.
func (s *serviceImpl) ListRunRows(ctx context.Context, sc Scope, runID int64, limit int) ([]DiagnosticRowData, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list run rows: run %d: %w", runID, err)
	}
	if sc.EffectiveOrg > 0 && run.OrganizationID != sc.EffectiveOrg {
		return nil, fmt.Errorf("list run rows: forbidden: run belongs to org %d", run.OrganizationID)
	}
	return s.repo.ListRunRows(ctx, runID, run.Workflow, limit)
}

// MarkReviewable marks a match run as ready for review.
func (s *serviceImpl) MarkReviewable(ctx context.Context, runID int64) error {
	return s.repo.SetState(ctx, runID, "reviewable", false)
}

// Rematch performs matching on a single row without rebuilding the index.
func (s *serviceImpl) Rematch(ctx context.Context, sc Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error) {
	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	pol, err := s.repo.ActivePolicy(ctx, string(sc.Workflow), sc.EffectiveOrg)
	if err != nil || pol.Version == "" {
		pol = productmatch.DefaultPolicy()
	}

	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		return matchflow.RowDecision{
			V:        1,
			State:    matchflow.StateUnavailable,
			Basis:    matchflow.BasisSimilarity,
			Reasons:  []matchflow.Reason{{Code: string(productmatch.ReasonCatalogUnavailable)}},
			Revision: rev,
			Versions: matchflow.Versions{
				Policy:          pol.Version,
				Normalizer:      productmatch.NormalizerVersion,
				CatalogRevision: catRev,
			},
			AI: matchflow.AIDecision{State: matchflow.AIStateOff},
		}, nil
	}

	dec := idx.Decide(&row, pol)
	dec.Revision = rev
	return dec, nil
}

// Revalidate checks if a decision made under an older revision needs revalidation.
func (s *serviceImpl) Revalidate(ctx context.Context, sc Scope, ref RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	if d.Versions.CatalogRevision != catRev {
		hasFlag := false
		for _, f := range d.Flags {
			if f == "catalog_changed" {
				hasFlag = true
				break
			}
		}
		if !hasFlag {
			d.Flags = append(d.Flags, "catalog_changed")
		}
	}

	return d, nil
}

func (s *serviceImpl) StartEnhancement(ctx context.Context, runID int64, rows []EnhanceRow) (EnhanceHandle, error) {
	return EnhanceHandle{}, ErrNotImplemented
}

func (s *serviceImpl) StopEnhancement(ctx context.Context, runID int64) error {
	return ErrNotImplemented
}
