package matching

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"


	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockApplier struct {
	applied map[RowRef]matchflow.RowDecision
}

func (a *mockApplier) ApplyAI(_ context.Context, ref RowRef, _ int, _ int64, d matchflow.RowDecision) (bool, error) {
	if a.applied == nil {
		a.applied = make(map[RowRef]matchflow.RowDecision)
	}
	a.applied[ref] = d
	return true, nil
}

func TestStartEnhancement_CohortRestriction(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	// 1. Create run
	run := &MatchRun{
		OrganizationID: 10,
		Workflow:       "vendor",
		State:          "reviewable",
	}
	_ = repo.CreateRun(ctx, run)

	// 2. Setup mock configuration with restricted cohorts: only Arabic standard vendor rows
	mockCfg := &AIConfiguration{
		ID:     "pm-cohort-test",
		Status: "qualified",
		QualifiedCohorts: []CohortSpec{
			{
				Workflow: "vendor",
				Language: "ar",
				RowType:  "standard",
			},
		},
	}
	repo.policies["vendor"] = productmatch.Policy{
		AI: productmatch.AIPolicy{
			ConfigID: "pm-cohort-test",
		},
	}
	// Add mock configuration to repo
	repo.mu.Lock()
	repo.aiCounters[run.ID] = make(map[string]any)
	repo.mu.Unlock()

	applier := &mockApplier{}

	// 3. Define 2 rows: one Arabic standard (qualified), one English standard (unqualified)
	rows := []EnhanceRow{
		{
			RowRef: RowRef{Table: "vendor_row", ID: 1},
			Row: productmatch.Row{
				Name:     "بنادول اكسترا اقراص",
				Category: "standard",
			},
			Decision: matchflow.RowDecision{
				Revision: 1,
				State:    matchflow.StateNeedsReview,
			},
		},
		{
			RowRef: RowRef{Table: "vendor_row", ID: 2},
			Row: productmatch.Row{
				Name:     "Panadol Extra Tablets", // English only -> unqualified
				Category: "standard",
			},
			Decision: matchflow.RowDecision{
				Revision: 1,
				State:    matchflow.StateNeedsReview,
			},
		},
	}

	// Override GetAIConfiguration on mockRepository
	origRepo := repo
	mockRepoWithCfg := &mockRepoWithSpecificConfig{
		mockRepository: origRepo,
		cfg:            mockCfg,
	}
	svcWithCfg := NewService(mockRepoWithCfg, nil, nil)
	svcWithCfg.RegisterApplier(Workflow("vendor"), applier)

	handle, err := svcWithCfg.StartEnhancement(ctx, run.ID, rows)
	assert.NoError(t, err)
	assert.Equal(t, run.ID, handle.RunID)

	// Verify unqualified row was marked ineligible with reason ai_rejected_unqualified
	unqualifiedDec, found := applier.applied[RowRef{Table: "vendor_row", ID: 2}]
	assert.True(t, found)
	assert.Equal(t, matchflow.AIStateIneligible, unqualifiedDec.AI.State)
	assert.Equal(t, "ineligible", unqualifiedDec.AI.Outcome)

	hasUnqualifiedReason := false
	for _, r := range unqualifiedDec.Reasons {
		if r.Code == string(productmatch.ReasonAIRejectedUnqualified) {
			hasUnqualifiedReason = true
			break
		}
	}
	assert.True(t, hasUnqualifiedReason, "must have reason ai_rejected_unqualified")

	// Verify unqualified count was reported
	assert.Equal(t, 1, origRepo.aiCounters[run.ID]["unqualified_cohort"])
}


type mockRepoWithSpecificConfig struct {
	*mockRepository
	cfg *AIConfiguration
}

func (m *mockRepoWithSpecificConfig) GetAIConfiguration(_ context.Context, _ string) (*AIConfiguration, error) {
	return m.cfg, nil
}
