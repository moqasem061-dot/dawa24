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

type mockKnowledgeRepo struct {
	orgItems  []Knowledge
	platItems []Knowledge
	upserted  []Knowledge
	revoked   []int64
	staled    []int64
}

func (m *mockKnowledgeRepo) LookupOrg(ctx context.Context, sigs []string) ([]Knowledge, error) {
	return m.orgItems, nil
}

func (m *mockKnowledgeRepo) LookupPlatform(ctx context.Context, sigs []string, orgID int64) ([]Knowledge, error) {
	return m.platItems, nil
}

func (m *mockKnowledgeRepo) Upsert(ctx context.Context, k Knowledge) error {
	m.upserted = append(m.upserted, k)
	return nil
}

func (m *mockKnowledgeRepo) Revoke(ctx context.Context, id int64, revokedBy string) error {
	m.revoked = append(m.revoked, id)
	return nil
}

func (m *mockKnowledgeRepo) MarkStale(ctx context.Context, ids []int64) error {
	m.staled = append(m.staled, ids...)
	return nil
}

func (m *mockKnowledgeRepo) List(ctx context.Context, f KnowledgeFilter) (Page[Knowledge], error) {
	return Page[Knowledge]{Items: m.orgItems, Total: len(m.orgItems)}, nil
}

func (m *mockKnowledgeRepo) LoadAliases(ctx context.Context, orgID int64) (map[string][]int64, error) {
	return nil, nil
}

func TestKnowledgeResolver_Precedence(t *testing.T) {
	pidConcor := int64(101)
	pidPanadol := int64(102)
	orgID := int64(10)
	sig := "sig_precedence"

	idx := testMasterIndex(1)
	concorProd, _ := idx.Lookup(pidConcor)
	concorFP := ProductFactFingerprint(concorProd)
	panadolProd, _ := idx.Lookup(pidPanadol)
	panadolFP := ProductFactFingerprint(panadolProd)

	now := time.Now()

	repo := &mockKnowledgeRepo{
		orgItems: []Knowledge{
			{
				ID:                    1,
				OrganizationID:        &orgID,
				SourceNamespace:       "*",
				QuestionSignature:     sig,
				Kind:                  "human_confirm",
				TargetProductID:       &pidConcor,
				TargetFactFingerprint: concorFP,
				Status:                "active",
				CreatedAt:             now.Add(-2 * time.Hour),
			},
			{
				ID:                    2,
				OrganizationID:        &orgID,
				SourceNamespace:       "*",
				QuestionSignature:     sig,
				Kind:                  "human_confirm",
				TargetProductID:       &pidPanadol,
				TargetFactFingerprint: panadolFP,
				Status:                "active",
				CreatedAt:             now.Add(-1 * time.Hour), // Newer confirm
			},
		},
	}

	kr := NewKnowledgeResolver(repo)
	sc := Scope{EffectiveOrg: orgID, Workflow: WorkflowVendor}
	row := &productmatch.Row{Number: 1, Name: "بانادول اكسترا 24 قرص"}
	pol := productmatch.DefaultPolicy()

	// 1. Newer human_confirm (Panadol) wins over older human_confirm (Concor)
	dec, resolved, err := kr.Resolve(context.Background(), sc, row, sig, idx, pol, nil)
	require.NoError(t, err)
	require.True(t, resolved)
	require.NotNil(t, dec)
	assert.Equal(t, pidPanadol, *dec.ProductID)
	assert.Equal(t, matchflow.BasisKnowledgeHuman, dec.Basis)

	// 2. human_reject beats human_confirm
	repo.orgItems = append(repo.orgItems, Knowledge{
		ID:                3,
		OrganizationID:    &orgID,
		SourceNamespace:   "*",
		QuestionSignature: sig,
		Kind:              "human_reject",
		Status:            "active",
		CreatedAt:         now.Add(-30 * time.Minute),
	})

	dec, resolved, err = kr.Resolve(context.Background(), sc, row, sig, idx, pol, nil)
	require.NoError(t, err)
	require.True(t, resolved)
	require.NotNil(t, dec)
	assert.Equal(t, matchflow.StateNoCandidate, dec.State)
	assert.Equal(t, matchflow.BasisKnowledgeHuman, dec.Basis)
}

func TestKnowledgeResolver_Staleness(t *testing.T) {
	pid := int64(101)
	orgID := int64(10)
	sig := "sig_stale"

	idx := testMasterIndex(1)

	// Entry with outdated fact fingerprint
	repo := &mockKnowledgeRepo{
		orgItems: []Knowledge{
			{
				ID:                    1,
				OrganizationID:        &orgID,
				SourceNamespace:       "*",
				QuestionSignature:     sig,
				Kind:                  "human_confirm",
				TargetProductID:       &pid,
				TargetFactFingerprint: "outdated_fingerprint_hash",
				PolicyVersion:         "v1.0",
				Status:                "active",
				CreatedAt:             time.Now(),
			},
		},
	}

	kr := NewKnowledgeResolver(repo)
	sc := Scope{EffectiveOrg: orgID, Workflow: WorkflowVendor}
	row := &productmatch.Row{Number: 1, Name: "كونكور 5 مجم 30 قرص"}
	pol := productmatch.DefaultPolicy()

	dec, resolved, err := kr.Resolve(context.Background(), sc, row, sig, idx, pol, nil)
	require.NoError(t, err)
	require.True(t, resolved)
	require.NotNil(t, dec)
	// Because target fact fingerprint changed, Gate marks it needs_review and adds stale_knowledge flag
	assert.Equal(t, matchflow.StateNeedsReview, dec.State)
	assert.Contains(t, dec.Flags, "stale_knowledge")
}

func TestKnowledgeResolver_AIDisabledIgnored(t *testing.T) {
	pid := int64(101)
	orgID := int64(10)
	sig := "sig_ai_off"

	idx := testMasterIndex(1)
	concorProd, _ := idx.Lookup(pid)

	repo := &mockKnowledgeRepo{
		orgItems: []Knowledge{
			{
				ID:                    1,
				OrganizationID:        &orgID,
				SourceNamespace:       "*",
				QuestionSignature:     sig,
				Kind:                  "ai_answer",
				TargetProductID:       &pid,
				TargetFactFingerprint: ProductFactFingerprint(concorProd),
				Status:                "active",
				CreatedAt:             time.Now(),
			},
		},
	}

	kr := NewKnowledgeResolver(repo)
	sc := Scope{EffectiveOrg: orgID, Workflow: WorkflowVendor}
	row := &productmatch.Row{Number: 1, Name: "كونكور 5 مجم 30 قرص"}

	// When policy has AI disabled, ai_answer is ignored (FR-057)
	pol := productmatch.DefaultPolicy()
	pol.AI.Enabled = false

	dec, resolved, err := kr.Resolve(context.Background(), sc, row, sig, idx, pol, nil)
	require.NoError(t, err)
	assert.False(t, resolved)
	assert.Nil(t, dec)
}

func TestKnowledgeResolver_NegativeAISubset(t *testing.T) {
	orgID := int64(10)
	sig := "sig_ai_neg"

	idx := testMasterIndex(1)

	// Stored negative AI answer with candidate IDs [101, 102]
	storedFP := CandidateSetFingerprint([]int64{101, 102})
	repo := &mockKnowledgeRepo{
		orgItems: []Knowledge{
			{
				ID:                      1,
				OrganizationID:          &orgID,
				SourceNamespace:         "*",
				QuestionSignature:       sig,
				Kind:                    "ai_answer",
				TargetProductID:         nil, // Negative answer
				CandidateSetFingerprint: &storedFP,
				Status:                  "active",
				CreatedAt:               time.Now(),
			},
		},
	}

	kr := NewKnowledgeResolver(repo)
	sc := Scope{EffectiveOrg: orgID, Workflow: WorkflowVendor}
	row := &productmatch.Row{Number: 1, Name: "علاج غير موجود"}
	pol := productmatch.DefaultPolicy()
	pol.AI.Enabled = true

	// 1. Current candidates [101, 102] match stored fingerprint => negative answer is recognized
	_, resolved, err := kr.Resolve(context.Background(), sc, row, sig, idx, pol, []int64{101, 102})
	require.NoError(t, err)
	// For negative AI answer, no target product to evaluate with Gate, returns unresolved to proceed with matching
	assert.False(t, resolved)

	// 2. Current candidate has a new product [101, 102, 103] => fingerprint mismatch => ignored
	_, resolved, err = kr.Resolve(context.Background(), sc, row, sig, idx, pol, []int64{101, 102, 103})
	require.NoError(t, err)
	assert.False(t, resolved)
}
