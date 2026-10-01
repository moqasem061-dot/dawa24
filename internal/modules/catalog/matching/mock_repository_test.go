package matching

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockRepository struct {
	mu          sync.Mutex
	revisions   map[string]int64
	policies    map[string]productmatch.Policy
	runs        map[int64]*MatchRun
	runStates   map[int64]string
	runFinished map[int64]bool
	nextRunID   int64
	attempts    []*AIAttemptRecord
	aiCounters  map[int64]map[string]any
	enhanceRows map[int64][]EnhanceRow
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		revisions:   make(map[string]int64),
		policies:    make(map[string]productmatch.Policy),
		runs:        make(map[int64]*MatchRun),
		runStates:   make(map[int64]string),
		runFinished: make(map[int64]bool),
		aiCounters:  make(map[int64]map[string]any),
		enhanceRows: make(map[int64][]EnhanceRow),
		nextRunID:   100,
	}
}

func (m *mockRepository) CurrentRevision(ctx context.Context, scope string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rev, ok := m.revisions[scope]; ok {
		return rev, nil
	}
	return 1, nil
}

func (m *mockRepository) ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pol, ok := m.policies[workflow]; ok {
		return pol, nil
	}
	return productmatch.DefaultPolicy(), nil
}

func (m *mockRepository) CreateRun(ctx context.Context, run *MatchRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextRunID++
	run.ID = m.nextRunID
	if run.HeartbeatAt.IsZero() {
		run.HeartbeatAt = time.Now()
	}
	m.runs[run.ID] = run
	m.runStates[run.ID] = run.State
	return nil
}

func (m *mockRepository) GetRun(ctx context.Context, runID int64) (*MatchRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[runID]; ok {
		return run, nil
	}
	return nil, fmt.Errorf("run not found: %d", runID)
}

func (m *mockRepository) UpdateCounts(ctx context.Context, runID int64, counts matchflow.RunCounts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[runID]; ok {
		run.Counts = counts
		return nil
	}
	return fmt.Errorf("run not found: %d", runID)
}

func (m *mockRepository) SetState(ctx context.Context, runID int64, state string, finished bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runStates[runID] = state
	m.runFinished[runID] = finished
	if run, ok := m.runs[runID]; ok {
		run.State = state
		if finished {
			now := time.Now()
			run.FinishedAt = &now
		}
	}
	return nil
}

func (m *mockRepository) Heartbeat(ctx context.Context, runID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[runID]; ok {
		run.HeartbeatAt = time.Now()
	}
	return nil
}

func (m *mockRepository) ListRunRows(ctx context.Context, runID int64, workflow string, limit int) ([]DiagnosticRowData, error) {
	return nil, nil
}

func (m *mockRepository) ListEnhanceRows(ctx context.Context, runID int64, workflow string) ([]EnhanceRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enhanceRows[runID], nil
}

func (m *mockRepository) CountActiveAIRuns(ctx context.Context, orgID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int
	for _, r := range m.runs {
		if r.OrganizationID == orgID && r.State == "enhancing" {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) UpdateAIRunCounters(ctx context.Context, runID int64, aiCounters map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aiCounters[runID] = aiCounters
	if r, ok := m.runs[runID]; ok {
		r.AI = aiCounters
	}
	return nil
}

func (m *mockRepository) RecordAIAttempt(ctx context.Context, attempt *AIAttemptRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts = append(m.attempts, attempt)
	return nil
}

func (m *mockRepository) PurgeOldAIAttempts(ctx context.Context, maxAge time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockRepository) LookupOrg(ctx context.Context, sigs []string) ([]Knowledge, error) {
	return nil, nil
}

func (m *mockRepository) LookupPlatform(ctx context.Context, sigs []string, orgID int64) ([]Knowledge, error) {
	return nil, nil
}

func (m *mockRepository) Upsert(ctx context.Context, k Knowledge) error {
	return nil
}

func (m *mockRepository) Revoke(ctx context.Context, id int64, revokedBy string) error {
	return nil
}

func (m *mockRepository) MarkStale(ctx context.Context, ids []int64) error {
	return nil
}

func (m *mockRepository) List(ctx context.Context, f KnowledgeFilter) (Page[Knowledge], error) {
	return Page[Knowledge]{}, nil
}

func (m *mockRepository) LoadAliases(ctx context.Context, orgID int64) (map[string][]int64, error) {
	return nil, nil
}

func (m *mockRepository) ListAIConfigurations(ctx context.Context) ([]AIConfiguration, error) {
	return nil, nil
}

func (m *mockRepository) GetAIConfiguration(ctx context.Context, id string) (*AIConfiguration, error) {
	return nil, nil
}

func (m *mockRepository) UpdateAIConfigurationStatus(ctx context.Context, id string, newStatus string, changedBy string, reason string) error {
	return nil
}

func (m *mockRepository) ResolvePolicyAssignment(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, string, error) {
	pol, err := m.ActivePolicy(ctx, workflow, orgID)
	return pol, "", err
}

func (m *mockRepository) GetPolicy(ctx context.Context, version string) (productmatch.Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.policies[version]; ok {
		return p, nil
	}
	return productmatch.DefaultPolicy(), nil
}

func (m *mockRepository) ListPolicies(ctx context.Context) ([]PolicyRecord, error) {
	return nil, nil
}

func (m *mockRepository) ListAssignments(ctx context.Context) ([]PolicyAssignment, error) {
	return nil, nil
}

func (m *mockRepository) UpdateAssignment(ctx context.Context, workflow, cohort, activeVersion string, shadowVersion *string, changedBy, reason string) error {
	return nil
}

func (m *mockRepository) RollbackAssignment(ctx context.Context, workflow, cohort, changedBy string) error {
	return nil
}

func (m *mockRepository) RecordShadowOutcomes(ctx context.Context, outcomes []ShadowOutcome) error {
	return nil
}

func (m *mockRepository) ListShadowOutcomes(ctx context.Context, runID int64) ([]ShadowOutcome, error) {
	return nil, nil
}
