package matching

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestDiagnose_TenantIsolation(t *testing.T) {
	repo := newMockRepository()
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return testMasterIndex(rev), nil
	})
	cache := NewIndexCache(builder)
	svc := NewService(repo, cache, nil)

	ctx := context.Background()
	run := &MatchRun{
		OrganizationID: 10,
		Workflow:       "vendor",
		SourceRevision: 1,
		State:          "reviewable",
		HeartbeatAt:    time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, repo.CreateRun(ctx, run))

	row := productmatch.Row{
		Number: 1,
		Name:   "كونكور 5 مجم",
	}
	dec := matchflow.RowDecision{
		V:        1,
		State:    matchflow.StateConfirmed,
		Basis:    matchflow.BasisExact,
		Revision: 1,
	}

	// 1. Cross-tenant access is rejected
	crossOrgScope := Scope{EffectiveOrg: 20, Workflow: "vendor"}
	_, err := svc.Diagnose(ctx, crossOrgScope, run.ID, row, dec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden: run belongs to org 10")

	// 2. Same-tenant access succeeds
	sameOrgScope := Scope{EffectiveOrg: 10, Workflow: "vendor"}
	diag, err := svc.Diagnose(ctx, sameOrgScope, run.ID, row, dec)
	require.NoError(t, err)
	assert.Equal(t, run.ID, diag.RunID)
	assert.Equal(t, int64(10), diag.OrganizationID)
}

func TestListRunRows_TenantIsolation(t *testing.T) {
	repo := newMockRepository()
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return testMasterIndex(rev), nil
	})
	cache := NewIndexCache(builder)
	svc := NewService(repo, cache, nil)

	ctx := context.Background()
	run := &MatchRun{
		OrganizationID: 10,
		Workflow:       "vendor",
		SourceRevision: 1,
		State:          "reviewable",
		HeartbeatAt:    time.Now(),
		CreatedAt:      time.Now(),
	}
	require.NoError(t, repo.CreateRun(ctx, run))

	// 1. Cross-tenant access is rejected
	crossOrgScope := Scope{EffectiveOrg: 20, Workflow: "vendor"}
	_, err := svc.ListRunRows(ctx, crossOrgScope, run.ID, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden: run belongs to org 10")

	// 2. Same-tenant access succeeds
	sameOrgScope := Scope{EffectiveOrg: 10, Workflow: "vendor"}
	rows, err := svc.ListRunRows(ctx, sameOrgScope, run.ID, 50)
	require.NoError(t, err)
	assert.Empty(t, rows) // Mock repository returns empty slice
}

func TestBuildDiagnosticRow_InferredFactsAndCandidates(t *testing.T) {
	idx := testMasterIndex(1)
	run := &MatchRun{
		ID:             42,
		OrganizationID: 10,
		Workflow:       "vendor",
		SourceRevision: 1,
		State:          "reviewable",
		CreatedBy:      "user_1",
		CreatedAt:      time.Now(),
		HeartbeatAt:    time.Now(),
	}

	row := productmatch.Row{
		Number:        1,
		Name:          "كونكور 5 مجم 30 قرص",
		SKU:           "CON-5",
		Barcode:       "6221234567890",
		DosageForm:    "قرص",
		Concentration: "5 mg",
		Manufacturer:  "Merck",
		Unit:          "30",
	}

	actorID := "reviewer_1"
	dec := matchflow.RowDecision{
		V:        1,
		State:    matchflow.StateConfirmed,
		Basis:    matchflow.BasisExact,
		Revision: 2,
		Actor: &matchflow.DecisionActor{
			UserID: actorID,
			At:     time.Now(),
			Corrections: []matchflow.Correction{
				{Attribute: "dosage_form", Value: "قرص"},
			},
		},
		Candidates: []matchflow.CandidateDecision{
			{
				ProductID: 101,
				Eligible:  true,
				Reasons:   []string{"exact_name_match"},
				Diff:      map[string]string{"dosage_form": "قرص"},
			},
		},
		Versions: matchflow.Versions{
			Policy:          "v1.0",
			Normalizer:      "v1.0",
			CatalogRevision: 1,
		},
		AI: matchflow.AIDecision{
			State:    matchflow.AIStateCompleted,
			Outcome:  "verified",
			Attempts: []int64{1},
		},
	}

	diag := BuildDiagnosticRow(idx, run, row, dec)

	// Validate original facts
	assert.Equal(t, "كونكور 5 مجم 30 قرص", diag.OriginalFacts["name"])
	assert.Equal(t, "CON-5", diag.OriginalFacts["sku"])
	assert.Equal(t, "6221234567890", diag.OriginalFacts["barcode"])
	assert.Equal(t, "قرص", diag.OriginalFacts["dosage_form"])
	assert.Equal(t, "5 mg", diag.OriginalFacts["concentration"])
	assert.Equal(t, "Merck", diag.OriginalFacts["manufacturer"])
	assert.Equal(t, "30", diag.OriginalFacts["unit"])

	// Validate inferred facts
	assert.NotEmpty(t, diag.InferredFacts["normalized"])
	assert.NotEmpty(t, diag.InferredFacts["brand_stem"])
	assert.NotEmpty(t, diag.InferredFacts["dose_text"])
	assert.NotEmpty(t, diag.InferredFacts["skeleton"])

	// Validate corrected facts
	require.Len(t, diag.CorrectedFacts, 1)
	assert.Equal(t, "dosage_form", diag.CorrectedFacts[0].Attribute)

	// Validate candidates
	require.Len(t, diag.Candidates, 1)
	assert.Equal(t, int64(101), diag.Candidates[0].ProductID)
	assert.Equal(t, "كونكور 5 مجم 30 قرص", diag.Candidates[0].NameAR)

	// Validate revision history
	require.Len(t, diag.RevisionHistory, 2)
	assert.Equal(t, 1, diag.RevisionHistory[0].Revision)
	assert.Equal(t, "initial_preparation", diag.RevisionHistory[0].Action)
	assert.Equal(t, 2, diag.RevisionHistory[1].Revision)
	assert.Equal(t, "human_review_correction", diag.RevisionHistory[1].Action)
	assert.Equal(t, actorID, diag.RevisionHistory[1].Actor)

	// Validate AI attempts
	require.Len(t, diag.AIAttempts, 1)
	assert.Equal(t, string(matchflow.AIStateCompleted), diag.AIAttempts[0].State)
	assert.Equal(t, "verified", diag.AIAttempts[0].Outcome)
}
