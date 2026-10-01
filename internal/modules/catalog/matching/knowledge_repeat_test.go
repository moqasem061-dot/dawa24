package matching

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type repeatTestRepo struct {
	mu         sync.Mutex
	revisions  map[string]int64
	policies   map[string]productmatch.Policy
	knowledge  map[int64]Knowledge
	runs       map[int64]*MatchRun
	currentOrg int64
	nextID     int64
}

func newRepeatTestRepo() *repeatTestRepo {
	return &repeatTestRepo{
		revisions: make(map[string]int64),
		policies:  make(map[string]productmatch.Policy),
		knowledge: make(map[int64]Knowledge),
		runs:      make(map[int64]*MatchRun),
		nextID:    1,
	}
}

func (r *repeatTestRepo) CurrentRevision(ctx context.Context, scope string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rev, ok := r.revisions[scope]; ok {
		return rev, nil
	}
	return 1, nil
}

func (r *repeatTestRepo) ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pol, ok := r.policies[workflow]; ok {
		return pol, nil
	}
	return productmatch.DefaultPolicy(), nil
}

func (r *repeatTestRepo) CreateRun(ctx context.Context, run *MatchRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.ID = r.nextID
	r.nextID++
	r.runs[run.ID] = run
	return nil
}

func (r *repeatTestRepo) GetRun(ctx context.Context, runID int64) (*MatchRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[runID], nil
}

func (r *repeatTestRepo) ListRunRows(ctx context.Context, runID int64, workflow string, limit int) ([]DiagnosticRowData, error) {
	return nil, nil
}

func (r *repeatTestRepo) UpdateCounts(ctx context.Context, runID int64, counts matchflow.RunCounts) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[runID]; ok {
		run.Counts = counts
	}
	return nil
}

func (r *repeatTestRepo) SetState(ctx context.Context, runID int64, state string, finished bool) error {
	return nil
}

func (r *repeatTestRepo) Heartbeat(ctx context.Context, runID int64) error { return nil }

func (r *repeatTestRepo) effectiveOrg(ctx context.Context) int64 {
	if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		return tid
	}
	return r.currentOrg
}

func (r *repeatTestRepo) LookupOrg(ctx context.Context, sigs []string) ([]Knowledge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	orgID := r.effectiveOrg(ctx)
	sigSet := make(map[string]bool, len(sigs))
	for _, s := range sigs {
		sigSet[s] = true
	}

	var results []Knowledge
	for _, k := range r.knowledge {
		if k.Status != "active" {
			continue
		}
		if k.OrganizationID != nil && *k.OrganizationID == orgID && sigSet[k.QuestionSignature] {
			results = append(results, k)
		}
	}
	return results, nil
}

func (r *repeatTestRepo) LookupPlatform(ctx context.Context, sigs []string, orgID int64) ([]Knowledge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sigSet := make(map[string]bool, len(sigs))
	for _, s := range sigs {
		sigSet[s] = true
	}

	var results []Knowledge
	for _, k := range r.knowledge {
		if k.Status != "active" {
			continue
		}
		if k.OrganizationID == nil && sigSet[k.QuestionSignature] {
			results = append(results, k)
		}
	}
	return results, nil
}

func (r *repeatTestRepo) Upsert(ctx context.Context, k Knowledge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if k.ID == 0 {
		k.ID = r.nextID
		r.nextID++
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	r.knowledge[k.ID] = k
	return nil
}

func (r *repeatTestRepo) Revoke(ctx context.Context, id int64, revokedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if k, ok := r.knowledge[id]; ok {
		k.Status = "revoked"
		r.knowledge[id] = k
	}
	return nil
}

func (r *repeatTestRepo) MarkStale(ctx context.Context, ids []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		if k, ok := r.knowledge[id]; ok {
			k.Status = "stale"
			r.knowledge[id] = k
		}
	}
	return nil
}

func (r *repeatTestRepo) List(ctx context.Context, f KnowledgeFilter) (Page[Knowledge], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Knowledge, 0, len(r.knowledge))
	for _, k := range r.knowledge {
		items = append(items, k)
	}
	return Page[Knowledge]{Items: items, Total: len(items)}, nil
}

func (r *repeatTestRepo) LoadAliases(ctx context.Context, orgID int64) (map[string][]int64, error) { return nil, nil }

func repeatCatalogueIndex() *productmatch.Index {
	prods := []productmatch.MasterProduct{
		{ID: 101, NameAR: "كونكور 5 مجم 30 قرص", NameEN: "Concor 5mg 30 tab", DosageForm: "قرص", Concentration: "5 mg", Unit: "30", Manufacturer: "Merck"},
		{ID: 102, NameAR: "بانادول اكسترا 24 قرص", NameEN: "Panadol Extra 24 tab", DosageForm: "قرص", Concentration: "500 mg", Unit: "24", Manufacturer: "GSK"},
		{ID: 103, NameAR: "اماريل 2 مجم 30 قرص", NameEN: "Amaryl 2mg 30 tab", DosageForm: "قرص", Concentration: "2 mg", Unit: "30", Manufacturer: "Sanofi"},
		{ID: 104, NameAR: "اوجمنتين 1 جم 14 قرص", NameEN: "Augmentin 1g 14 tab", DosageForm: "قرص", Concentration: "1 g", Unit: "14", Manufacturer: "GSK"},
		{ID: 105, NameAR: "بروفين 400 مجم 30 قرص", NameEN: "Brufen 400mg 30 tab", DosageForm: "قرص", Concentration: "400 mg", Unit: "30", Manufacturer: "Abbott"},
		{ID: 106, NameAR: "كتافلام 50 مجم 20 قرص", NameEN: "Cataflam 50mg 20 tab", DosageForm: "قرص", Concentration: "50 mg", Unit: "20", Manufacturer: "Novartis"},
		{ID: 107, NameAR: "جلوكوفاج 1000 مجم 30 قرص", NameEN: "Glucophage 1000mg 30 tab", DosageForm: "قرص", Concentration: "1000 mg", Unit: "30", Manufacturer: "Merck"},
		{ID: 108, NameAR: "ليبيتور 20 مجم 28 قرص", NameEN: "Lipitor 20mg 28 tab", DosageForm: "قرص", Concentration: "20 mg", Unit: "28", Manufacturer: "Pfizer"},
		{ID: 109, NameAR: "نيكسيوم 40 مجم 14 كبسول", NameEN: "Nexium 40mg 14 cap", DosageForm: "كبسول", Concentration: "40 mg", Unit: "14", Manufacturer: "AstraZeneca"},
		{ID: 110, NameAR: "زيثروماكس 500 مجم 3 كبسولات", NameEN: "Zithromax 500mg 3 cap", DosageForm: "كبسول", Concentration: "500 mg", Unit: "3", Manufacturer: "Pfizer"},
	}
	idx := productmatch.NewIndex(prods)
	idx.WithCatalogRevision(1)
	return idx
}

// TestRepeatFileKnowledge_SC012 verifies SC-012:
// Re-running an unchanged input file with 10 human-confirmed questions achieves
// >= 95% resolution with 0 AI calls, and negative cases (revoke, target change, policy change, cross-org) are blocked.
func TestRepeatFileKnowledge_SC012(t *testing.T) {
	const orgID = int64(42)
	ctx := database.WithTenant(context.Background(), orgID)
	repo := newRepeatTestRepo()
	repo.currentOrg = orgID

	idx := repeatCatalogueIndex()
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return idx, nil
	})
	svc := NewService(repo, NewIndexCache(builder), nil)

	// Define 10 input rows (slightly different raw phrasing simulating supplier price list)
	rawRows := []productmatch.Row{
		{Number: 1, Name: "concor 5mg 30 tab"},
		{Number: 2, Name: "panadol extra 24 tabs"},
		{Number: 3, Name: "amaryl 2mg tab 30"},
		{Number: 4, Name: "augmentin 1 gm 14 tab"},
		{Number: 5, Name: "brufen 400 mg 30 tab"},
		{Number: 6, Name: "cataflam 50 mg 20 tab"},
		{Number: 7, Name: "glucophage 1000 mg 30 tab"},
		{Number: 8, Name: "lipitor 20 mg 28 tab"},
		{Number: 9, Name: "nexium 40 mg 14 cap"},
		{Number: 10, Name: "zithromax 500 mg 3 cap"},
	}

	pids := []int64{101, 102, 103, 104, 105, 106, 107, 108, 109, 110}

	// Pre-populate 10 human-confirmed match decisions in match_knowledge for org 42
	knowledgeIDs := make([]int64, 10)
	for i := 0; i < 10; i++ {
		row := rawRows[i]
		rowFacts := productmatch.ExtractRowFacts(&row, nil)
		sig := productmatch.QuestionSignature(rowFacts, row.Name, productmatch.NormalizerVersion)

		targetProd, ok := idx.Lookup(pids[i])
		if !ok || targetProd == nil {
			t.Fatalf("target product %d missing from test index", pids[i])
		}
		fp := ProductFactFingerprint(targetProd)

		kOrg := orgID
		pid := pids[i]
		k := Knowledge{
			ID:                    int64(i + 1),
			OrganizationID:        &kOrg,
			SourceNamespace:       string(WorkflowVendor),
			QuestionSignature:     sig,
			Kind:                  "human_confirm",
			TargetProductID:       &pid,
			TargetFactFingerprint: fp,
			PolicyVersion:         "v1",
			NormalizerVersion:     productmatch.NormalizerVersion,
			DisplayText:           row.Name,
			Status:                "active",
			CreatedAt:             time.Now().Add(-1 * time.Hour),
		}
		_ = repo.Upsert(ctx, k)
		knowledgeIDs[i] = k.ID
	}

	req := PrepareRequest{
		Scope:       Scope{EffectiveOrg: orgID, Workflow: WorkflowVendor, EntryPoint: "vendor.import"},
		ConsumerRef: "test-file-repeat-1",
		SourceRev:   1,
		Rows:        rawRows,
	}

	// 1. Positive repeat file execution (SC-012)
	res, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if len(res.Decisions) != 10 {
		t.Fatalf("expected 10 decisions, got %d", len(res.Decisions))
	}

	confirmedCount := 0
	for i, dec := range res.Decisions {
		if dec.State != matchflow.StateAccepted {
			t.Errorf("row %d: expected StateAccepted, got %s", i+1, dec.State)
		}
		if dec.Basis != matchflow.BasisKnowledgeHuman {
			t.Errorf("row %d: expected BasisKnowledgeHuman, got %s", i+1, dec.Basis)
		}
		if dec.ProductID == nil || *dec.ProductID != pids[i] {
			t.Errorf("row %d: expected product %d, got %v", i+1, pids[i], dec.ProductID)
		}
		if dec.AI.State != matchflow.AIStateOff {
			t.Errorf("row %d: expected 0 AI calls (AIStateOff), got %s", i+1, dec.AI.State)
		}
		if dec.Basis == matchflow.BasisKnowledgeHuman && dec.AI.State == matchflow.AIStateOff {
			confirmedCount++
		}
	}

	ratio := float64(confirmedCount) / float64(len(rawRows))
	if ratio < 0.95 {
		t.Fatalf("expected >= 95%% zero-AI human-confirmed reuse, got %.1f%% (%d/10)", ratio*100, confirmedCount)
	}
	if res.Summary.KnowledgeReuse != 10 {
		t.Errorf("expected KnowledgeReuse=10, got %d", res.Summary.KnowledgeReuse)
	}

	// 2. Negative test case: Revocation blocks reuse
	sc := req.Scope
	if err := svc.RevokeKnowledge(ctx, sc, knowledgeIDs[0]); err != nil {
		t.Fatalf("RevokeKnowledge failed: %v", err)
	}
	resRevoked, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare after revoke failed: %v", err)
	}
	if resRevoked.Decisions[0].Basis == matchflow.BasisKnowledgeHuman {
		t.Errorf("row 1: revoked knowledge must not be used as BasisKnowledgeHuman")
	}

	// 3. Negative test case: Target change (fact drift) blocks reuse
	// Modify knowledge record 2 to point to a drifted fingerprint
	k2 := repo.knowledge[knowledgeIDs[1]]
	k2.TargetFactFingerprint = "drifted_fact_hash_xyz"
	_ = repo.Upsert(ctx, k2)

	resDrift, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare after drift failed: %v", err)
	}
	if resDrift.Decisions[1].State == matchflow.StateAccepted {
		t.Errorf("row 2: drifted target fact fingerprint must block acceptance")
	}
	hasStaleFlag := false
	for _, f := range resDrift.Decisions[1].Flags {
		if f == "stale_knowledge" {
			hasStaleFlag = true
		}
	}
	if !hasStaleFlag {
		t.Errorf("row 2: expected stale_knowledge flag, got %v", resDrift.Decisions[1].Flags)
	}

	// 4. Negative test case: Policy change blocks incompatible knowledge
	incompatPol := productmatch.DefaultPolicy()
	incompatPol.Version = "v2"
	incompatPol.KnowledgeCompat = []string{"v2"} // Only v2 allowed, knowledge has v1
	repo.policies[string(WorkflowVendor)] = incompatPol

	resPol, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare after policy change failed: %v", err)
	}
	// With policy incompatibility, remaining rows must not be accepted as fresh knowledge
	if resPol.Decisions[2].State == matchflow.StateAccepted {
		t.Errorf("row 3: incompatible policy version must block acceptance")
	}
	hasStaleFlag3 := false
	for _, f := range resPol.Decisions[2].Flags {
		if f == "stale_knowledge" {
			hasStaleFlag3 = true
		}
	}
	if !hasStaleFlag3 {
		t.Errorf("row 3: expected stale_knowledge flag on row 3, got %v", resPol.Decisions[2].Flags)
	}

	// Reset policy for remaining checks
	repo.policies[string(WorkflowVendor)] = productmatch.DefaultPolicy()

	// 5. Negative test case: Cross-org access is blocked (tenant isolation)
	const otherOrgID = int64(99)
	otherCtx := database.WithTenant(context.Background(), otherOrgID)
	repo.currentOrg = otherOrgID

	otherReq := PrepareRequest{
		Scope:       Scope{EffectiveOrg: otherOrgID, Workflow: WorkflowVendor, EntryPoint: "vendor.import"},
		ConsumerRef: "test-file-repeat-other-org",
		SourceRev:   1,
		Rows:        rawRows,
	}

	resOther, err := svc.Prepare(otherCtx, otherReq)
	if err != nil {
		t.Fatalf("Prepare for other org failed: %v", err)
	}
	if resOther.Summary.KnowledgeReuse != 0 {
		t.Errorf("expected 0 knowledge reuse for cross-org access, got %d", resOther.Summary.KnowledgeReuse)
	}
	for i, dec := range resOther.Decisions {
		if dec.Basis == matchflow.BasisKnowledgeHuman {
			t.Errorf("row %d: other org must not access org 42's human-confirmed knowledge", i+1)
		}
	}
}
