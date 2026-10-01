package matching

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func testMasterIndex(rev int64) *productmatch.Index {
	prods := []productmatch.MasterProduct{
		{ID: 101, NameAR: "كونكور 5 مجم 30 قرص", NameEN: "Concor 5mg 30 tab", DosageForm: "قرص", Concentration: "5 mg", Unit: "30", Manufacturer: "Merck"},
		{ID: 102, NameAR: "بانادول اكسترا 24 قرص", NameEN: "Panadol Extra 24 tab", DosageForm: "قرص", Concentration: "500 mg", Unit: "24", Manufacturer: "GSK"},
	}
	idx := productmatch.NewIndex(prods)
	idx.WithCatalogRevision(rev)
	return idx
}

func TestService_Prepare_Dedup(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	var builds int32
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		atomic.AddInt32(&builds, 1)
		return testMasterIndex(rev), nil
	})
	svc := NewService(repo, NewIndexCache(builder), nil)

	req := PrepareRequest{
		Scope:       Scope{EffectiveOrg: 42, Workflow: WorkflowVendor, EntryPoint: "test"},
		ConsumerRef: "ingest.imports:1",
		SourceRev:   1,
		Rows: []productmatch.Row{
			{Name: "بانادول اكسترا 24 قرص"},
			{Name: "بانادول اكسترا 24 قرص"},
			{Name: "كونكور 5 مجم 30 قرص"},
			{Name: "بانادول اكسترا 24 قرص"},
		},
	}

	res, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Decisions) != 4 {
		t.Fatalf("expected 4 decisions, got %d", len(res.Decisions))
	}
	if res.Summary.DistinctQuestions != 2 {
		t.Errorf("expected 2 distinct questions, got %d", res.Summary.DistinctQuestions)
	}
	if res.Summary.PrimaryTotal() != 4 || res.Summary.Total != 4 {
		t.Errorf("expected primary total 4, got %d (total: %d)", res.Summary.PrimaryTotal(), res.Summary.Total)
	}

	for _, idx := range []int{0, 1, 3} {
		if res.Decisions[idx].ProductID == nil || *res.Decisions[idx].ProductID != 102 {
			t.Errorf("row %d expected product 102, got %v", idx, res.Decisions[idx].ProductID)
		}
	}
	if res.Decisions[2].ProductID == nil || *res.Decisions[2].ProductID != 101 {
		t.Errorf("row 2 expected product 101, got %v", res.Decisions[2].ProductID)
	}
}

func TestService_Prepare_PanicIsolation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return testMasterIndex(rev), nil
	})
	cache := NewIndexCache(builder)

	impl := &serviceImpl{
		repo:       repo,
		indexCache: cache,
		decideFn: func(idx *productmatch.Index, row *productmatch.Row, pol productmatch.Policy) matchflow.RowDecision {
			if row.Name == "PANIC_ROW" {
				panic("simulated engine panic on uninterpretable numeric")
			}
			return idx.Decide(row, pol)
		},
	}

	req := PrepareRequest{
		Scope:     Scope{EffectiveOrg: 10, Workflow: WorkflowVendor},
		SourceRev: 2,
		Rows: []productmatch.Row{
			{Name: "بانادول اكسترا 24 قرص"},
			{Name: "PANIC_ROW"},
			{Name: "كونكور 5 مجم 30 قرص"},
		},
	}

	res, err := impl.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare should not fail on panic: %v", err)
	}
	if len(res.Decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(res.Decisions))
	}

	if res.Decisions[0].State != matchflow.StateAccepted {
		t.Errorf("row 0 should be accepted, got %s", res.Decisions[0].State)
	}
	d1 := res.Decisions[1]
	if d1.State != matchflow.StateInvalidInput {
		t.Errorf("row 1 expected invalid_input, got %s", d1.State)
	}
	if len(d1.Reasons) == 0 || d1.Reasons[0].Code != string(productmatch.ReasonNumericUninterpretable) {
		t.Errorf("row 1 expected numeric_uninterpretable reason, got %v", d1.Reasons)
	}
	if res.Decisions[2].State != matchflow.StateAccepted {
		t.Errorf("row 2 should be accepted, got %s", res.Decisions[2].State)
	}

	if res.Summary.PrimaryTotal() != 3 {
		t.Errorf("expected primary total 3, got %d", res.Summary.PrimaryTotal())
	}
	if res.Summary.InvalidInput != 1 || res.Summary.Accepted != 2 {
		t.Errorf("unexpected counts: invalid=%d, accepted=%d", res.Summary.InvalidInput, res.Summary.Accepted)
	}
}

func TestService_Prepare_CatalogueUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		return nil, errors.New("database connection refused")
	})
	svc := NewService(repo, NewIndexCache(builder), nil)

	req := PrepareRequest{
		Scope:     Scope{EffectiveOrg: 15, Workflow: WorkflowVendor},
		SourceRev: 3,
		Rows: []productmatch.Row{
			{Name: "بانادول اكسترا"},
			{Name: "كونكور 5 مجم"},
		},
	}

	res, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, d := range res.Decisions {
		if d.State != matchflow.StateUnavailable {
			t.Errorf("row %d expected unavailable, got %s", i, d.State)
		}
		if d.State == matchflow.StateNoCandidate {
			t.Errorf("row %d must not be no_candidate", i)
		}
		if len(d.Reasons) == 0 || d.Reasons[0].Code != string(productmatch.ReasonCatalogUnavailable) {
			t.Errorf("row %d expected catalog_unavailable, got %v", i, d.Reasons)
		}
	}

	if res.Summary.Unavailable != 2 || res.Summary.NoCandidate != 0 {
		t.Errorf("expected unavailable=2, no_candidate=0, got unavailable=%d, no_candidate=%d",
			res.Summary.Unavailable, res.Summary.NoCandidate)
	}
	if res.Summary.PrimaryTotal() != 2 {
		t.Errorf("expected primary total 2, got %d", res.Summary.PrimaryTotal())
	}
}

func TestService_Rematch_And_Revalidate(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.revisions["master"] = 5

	var builds int32
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		atomic.AddInt32(&builds, 1)
		return testMasterIndex(rev), nil
	})
	svc := NewService(repo, NewIndexCache(builder), nil)
	sc := Scope{EffectiveOrg: 7, Workflow: WorkflowVendor}

	row := productmatch.Row{Name: "بانادول اكسترا 24 قرص"}
	dec, err := svc.Rematch(ctx, sc, row, 10)
	if err != nil {
		t.Fatalf("Rematch failed: %v", err)
	}
	if dec.State != matchflow.StateAccepted || dec.Revision != 10 {
		t.Errorf("expected accepted with rev 10, got %s rev %d", dec.State, dec.Revision)
	}

	// Rematch again should hit cache without rebuilding
	if _, err := svc.Rematch(ctx, sc, row, 11); err != nil {
		t.Fatalf("second Rematch failed: %v", err)
	}
	if atomic.LoadInt32(&builds) != 1 {
		t.Errorf("expected 1 index build across rematches, got %d", builds)
	}

	// MarkReviewable
	if err := svc.MarkReviewable(ctx, 101); err != nil {
		t.Fatalf("MarkReviewable failed: %v", err)
	}
	if repo.runStates[101] != "reviewable" {
		t.Errorf("expected reviewable, got %s", repo.runStates[101])
	}

	// Revalidate: same revision -> no flag
	revMatch, err := svc.Revalidate(ctx, sc, RowRef{ID: 1}, dec)
	if err != nil {
		t.Fatalf("Revalidate failed: %v", err)
	}
	for _, f := range revMatch.Flags {
		if f == "catalog_changed" {
			t.Errorf("catalog_changed should not be set when revision matches")
		}
	}

	// Revalidate: older revision -> catalog_changed flag added
	decOlder := dec
	decOlder.Versions.CatalogRevision = 3
	revOld, err := svc.Revalidate(ctx, sc, RowRef{ID: 2}, decOlder)
	if err != nil {
		t.Fatalf("Revalidate older failed: %v", err)
	}
	found := false
	for _, f := range revOld.Flags {
		if f == "catalog_changed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected catalog_changed flag when revision differs")
	}
}

func TestService_UnimplementedMethods(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, NewIndexCache(nil), nil)
	sc := Scope{EffectiveOrg: 1}

	check := func(name string, err error) {
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("%s expected ErrNotImplemented, got %v", name, err)
		}
	}
	_, err := svc.StartEnhancement(ctx, 1, nil)
	check("StartEnhancement", err)
	check("StopEnhancement", svc.StopEnhancement(ctx, 1))
	check("Remember", svc.Remember(ctx, sc, KnowledgeWrite{}))
	_, err = svc.ListKnowledge(ctx, sc, KnowledgeFilter{})
	check("ListKnowledge", err)
	check("RevokeKnowledge", svc.RevokeKnowledge(ctx, sc, 1))
	check("PromoteKnowledge", svc.PromoteKnowledge(ctx, sc, 1))
}
