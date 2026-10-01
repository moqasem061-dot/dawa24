package matching

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

var (
	// ErrMaxActiveAIRunsReached is returned when an organization attempts to start a 3rd active AI run.
	ErrMaxActiveAIRunsReached = errors.New("matching: maximum active AI runs reached for organization (limit 2)")
	// ErrRunNotFound is returned when the specified match run does not exist.
	ErrRunNotFound = errors.New("matching: match run not found")
	// ErrApplierNotFound is returned when no Applier is registered for a workflow.
	ErrApplierNotFound = errors.New("matching: consumer applier not registered for workflow")
)

// ActiveAIRunLimit is the maximum concurrent AI runs allowed per organization (US9 T125, R9).
const ActiveAIRunLimit = 2

// StartEnhancement initiates asynchronous AI matching for a run (US9 T125).
// Enforces <= 2 active AI runs per organization at enqueue, and filters by qualified cohorts (US10 T137).
func (s *serviceImpl) StartEnhancement(ctx context.Context, runID int64, rows []EnhanceRow) (EnhanceHandle, error) {
	if s.repo == nil {
		return EnhanceHandle{}, ErrNotImplemented
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return EnhanceHandle{}, fmt.Errorf("%w: %v", ErrRunNotFound, err)
	}

	activeCount, err := s.repo.CountActiveAIRuns(ctx, run.OrganizationID)
	if err != nil {
		return EnhanceHandle{}, fmt.Errorf("check active AI runs: %w", err)
	}
	if activeCount >= ActiveAIRunLimit {
		return EnhanceHandle{}, ErrMaxActiveAIRunsReached
	}

	now := time.Now().UTC()
	if err := s.repo.SetState(ctx, runID, "enhancing", false); err != nil {
		return EnhanceHandle{}, fmt.Errorf("set run state enhancing: %w", err)
	}

	// 1. Resolve configuration and qualified cohorts (US10 T137)
	var qualifiedCohorts []CohortSpec
	if pol, pErr := s.repo.ActivePolicy(ctx, run.Workflow, run.OrganizationID); pErr == nil {
		configID := pol.AI.ConfigID
		if configID == "" {
			configID = "pm-default-v8"
		}
		if cfg, cErr := s.repo.GetAIConfiguration(ctx, configID); cErr == nil && cfg != nil {
			qualifiedCohorts = cfg.QualifiedCohorts
		}
	}
	if len(qualifiedCohorts) == 0 {
		if cfg, cErr := s.repo.GetAIConfiguration(ctx, "pm-default-v8"); cErr == nil && cfg != nil {
			qualifiedCohorts = cfg.QualifiedCohorts
		}
	}

	applier := s.GetApplier(Workflow(run.Workflow))

	// 2. Filter rows by qualified cohorts (FR-068, US10 T137)
	var qualifiedRows []EnhanceRow
	var unqualifiedCount int
	for _, r := range rows {
		lang := detectLanguage(r.Row.Name)
		rowType := detectRowType(r.Row)
		if isCohortQualified(qualifiedCohorts, run.Workflow, lang, rowType) {
			qualifiedRows = append(qualifiedRows, r)
		} else {
			unqualifiedCount++
			newDec := r.Decision
			newDec.AI.State = matchflow.AIStateIneligible
			newDec.AI.Outcome = "ineligible"
			newDec.Reasons = append(newDec.Reasons, matchflow.Reason{
				Code: string(productmatch.ReasonAIRejectedUnqualified),
			})
			if applier != nil {

				_, _ = applier.ApplyAI(ctx, r.RowRef, r.Decision.Revision, runID, newDec)
			}
		}
	}

	if unqualifiedCount > 0 {
		_ = s.repo.UpdateAIRunCounters(ctx, runID, map[string]any{
			"unqualified_cohort": unqualifiedCount,
		})
	}

	if len(rows) > 0 {
		s.activeRowsMu.Lock()
		s.activeRows[runID] = qualifiedRows
		s.activeRowsMu.Unlock()
	}

	// If all rows were unqualified, no background job is needed; mark reviewable
	if len(rows) > 0 && len(qualifiedRows) == 0 {
		_ = s.repo.SetState(ctx, runID, "reviewable", true)
		return EnhanceHandle{
			RunID:     runID,
			StartedAt: now,
		}, nil
	}

	if s.queueClient != nil {
		_, err = s.queueClient.Enqueue(ctx, queue.MatchEnhanceArgs{
			RunID:          runID,
			OrganizationID: run.OrganizationID,
			EnqueuedAt:     now,
		}, nil)
		if err != nil {
			_ = s.repo.SetState(ctx, runID, "reviewable", false)
			return EnhanceHandle{}, fmt.Errorf("enqueue match enhance job: %w", err)
		}
	}

	return EnhanceHandle{
		RunID:     runID,
		StartedAt: now,
	}, nil
}

// StopEnhancement cancels an in-flight AI matching run (US9 T125).
func (s *serviceImpl) StopEnhancement(ctx context.Context, runID int64) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	s.activeRowsMu.Lock()
	delete(s.activeRows, runID)
	s.activeRowsMu.Unlock()
	return s.repo.SetState(ctx, runID, "cancelled", true)
}

// RegisterApplier registers a consumer's Applier for AI result application.
func (s *serviceImpl) RegisterApplier(w Workflow, a Applier) {
	s.appliersMu.Lock()
	defer s.appliersMu.Unlock()
	if s.appliers == nil {
		s.appliers = make(map[Workflow]Applier)
	}
	s.appliers[w] = a
}

// GetApplier retrieves the consumer Applier for the workflow.
func (s *serviceImpl) GetApplier(w Workflow) Applier {
	s.appliersMu.RLock()
	defer s.appliersMu.RUnlock()
	return s.appliers[w]
}

func detectLanguage(name string) string {
	hasArabic := false
	hasLatin := false
	for _, r := range name {
		if unicode.In(r, unicode.Arabic) {
			hasArabic = true
		} else if unicode.In(r, unicode.Latin) {
			hasLatin = true
		}
	}
	if hasArabic && hasLatin {
		return "mixed"
	}
	if hasArabic {
		return "ar"
	}
	if hasLatin {
		return "en"
	}
	return "ar"
}

func detectRowType(row productmatch.Row) string {
	if row.Category != "" {
		return row.Category
	}
	return "standard"
}

func isCohortQualified(cohorts []CohortSpec, workflow, language, rowType string) bool {
	if len(cohorts) == 0 {
		return false
	}
	for _, c := range cohorts {
		wfMatch := c.Workflow == "*" || strings.EqualFold(c.Workflow, workflow)
		langMatch := c.Language == "*" || strings.EqualFold(c.Language, language) || (c.Language == "ar" && language == "mixed")
		rtMatch := c.RowType == "*" || strings.EqualFold(c.RowType, rowType)
		if wfMatch && langMatch && rtMatch {
			return true
		}
	}
	return false
}
