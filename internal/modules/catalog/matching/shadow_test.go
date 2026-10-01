package matching

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type shadowMockRepo struct {
	*mockRepository
	recordedOutcomes []ShadowOutcome
	shadowVersion    string
}

func (s *shadowMockRepo) ResolvePolicyAssignment(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, string, error) {
	pol, err := s.ActivePolicy(ctx, workflow, orgID)
	return pol, s.shadowVersion, err
}

func (s *shadowMockRepo) RecordShadowOutcomes(ctx context.Context, outcomes []ShadowOutcome) error {
	s.recordedOutcomes = append(s.recordedOutcomes, outcomes...)
	return nil
}

func (s *shadowMockRepo) ListShadowOutcomes(ctx context.Context, runID int64) ([]ShadowOutcome, error) {
	var res []ShadowOutcome
	for _, o := range s.recordedOutcomes {
		if o.RunID == runID {
			res = append(res, o)
		}
	}
	return res, nil
}

func TestShadowExecution_TriggersAndRecordsOutcomes(t *testing.T) {
	baseRepo := newMockRepository()
	shadowRepo := &shadowMockRepo{
		mockRepository: baseRepo,
		shadowVersion:  "pm-shadow-v2",
	}

	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return testMasterIndex(rev), nil
	})
	cache := NewIndexCache(builder)
	svc := NewService(shadowRepo, cache, nil)

	ctx := context.Background()
	req := PrepareRequest{
		Scope:       Scope{EffectiveOrg: 42, Workflow: WorkflowVendor, EntryPoint: "vendor_import"},
		ConsumerRef: "import_42",
		SourceRev:   1,
		Rows: []productmatch.Row{
			{Number: 1, Name: "كونكور 5 مجم"},
			{Number: 2, Name: "أسبوسيد 75 مجم"},
		},
	}

	res, err := svc.Prepare(ctx, req)
	require.NoError(t, err)
	assert.True(t, res.RunID > 0)
	assert.Len(t, res.Decisions, 2)

	// Since queueClient is nil, Prepare invokes RunShadow in a background goroutine.
	// Allow brief time for the goroutine to finish writing shadow outcomes.
	require.Eventually(t, func() bool {
		return len(shadowRepo.recordedOutcomes) == 2
	}, 1*time.Second, 20*time.Millisecond, "expected 2 shadow outcomes recorded")

	for _, o := range shadowRepo.recordedOutcomes {
		assert.Equal(t, res.RunID, o.RunID)
		assert.Equal(t, int64(42), o.OrganizationID)
		assert.Equal(t, "pm-shadow-v2", o.ShadowPolicyVersion)
		assert.NotEmpty(t, o.RowRef)
		assert.NotEmpty(t, o.Decision.State)
	}
}

func TestShadowExecution_NoShadowWhenVersionEmpty(t *testing.T) {
	baseRepo := newMockRepository()
	shadowRepo := &shadowMockRepo{
		mockRepository: baseRepo,
		shadowVersion:  "", // No shadow version assigned
	}

	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return testMasterIndex(rev), nil
	})
	cache := NewIndexCache(builder)
	svc := NewService(shadowRepo, cache, nil)

	ctx := context.Background()
	req := PrepareRequest{
		Scope:       Scope{EffectiveOrg: 42, Workflow: WorkflowVendor, EntryPoint: "vendor_import"},
		ConsumerRef: "import_42",
		SourceRev:   1,
		Rows: []productmatch.Row{
			{Number: 1, Name: "كونكور 5 مجم"},
		},
	}

	res, err := svc.Prepare(ctx, req)
	require.NoError(t, err)
	assert.True(t, res.RunID > 0)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, shadowRepo.recordedOutcomes, "no shadow outcomes should be recorded when shadow_version is empty")
}
