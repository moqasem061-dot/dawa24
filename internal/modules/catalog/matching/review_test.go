package matching

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func newTestReviewService() (Service, *mockRepository) {
	repo := newMockRepository()
	builder := IndexBuilderFunc(func(ctx context.Context, scope string, rev int64, normVer string) (*productmatch.Index, error) {
		prods := []productmatch.MasterProduct{
			{
				ID:            101,
				NameAR:        "كونكور 5 مجم 30 قرص",
				NameEN:        "Concor 5mg 30 tab",
				DosageForm:    "قرص",
				Concentration: "5 mg",
				Unit:          "30",
				Manufacturer:  "Merck",
				PublicPrice:   "50.00",
			},
			{
				ID:            102,
				NameAR:        "بانادول اكسترا 24 قرص",
				NameEN:        "Panadol Extra 24 tab",
				DosageForm:    "قرص",
				Concentration: "500 mg",
				Unit:          "24",
				Manufacturer:  "GSK",
				PublicPrice:   "35.00",
			},
			{
				ID:            103,
				NameAR:        "كونكور 10 مجم 30 قرص",
				NameEN:        "Concor 10mg 30 tab",
				DosageForm:    "قرص",
				Concentration: "10 mg",
				Unit:          "30",
				Manufacturer:  "Merck",
				PublicPrice:   "75.00",
			},
		}
		idx := productmatch.NewIndex(prods)
		idx.WithCatalogRevision(rev)
		return idx, nil
	})
	return NewService(repo, NewIndexCache(builder), nil), repo
}

func TestReview_OptimisticConcurrency_Mismatch(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor, ActorUserID: 99}

	action := ReviewAction{
		ExpectedRev: 1,
		CurrentDec: matchflow.RowDecision{
			Revision: 2, // Concurrent edit occurred
		},
		Action:   "select",
		TargetID: 101,
		Row: productmatch.Row{
			Name: "كونكور 5 مجم 30 قرص",
		},
	}

	_, err := svc.Review(ctx, sc, action)
	if err == nil {
		t.Fatal("expected ErrRevisionConflict on revision mismatch, got nil")
	}
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected ErrRevisionConflict, got: %v", err)
	}

	// Now with matching revisions
	action.ExpectedRev = 2
	dec, err := svc.Review(ctx, sc, action)
	if err != nil {
		t.Fatalf("unexpected error when revisions match: %v", err)
	}
	if dec.Revision != 3 {
		t.Fatalf("expected incremented revision 3, got %d", dec.Revision)
	}
}

func TestReview_HardConflictRejection_WithoutCorrection(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor, ActorUserID: 99}

	// Row says 10mg, reviewer attempts to select 5mg (ID 101) without corrections
	action := ReviewAction{
		ExpectedRev: 1,
		CurrentDec: matchflow.RowDecision{
			Revision: 1,
		},
		Action:   "select",
		TargetID: 101,
		Row: productmatch.Row{
			Name: "كونكور 10 مجم 30 قرص",
		},
	}

	_, err := svc.Review(ctx, sc, action)
	if err == nil {
		t.Fatal("expected ErrHardConflict on strength conflict without correction, got nil")
	}
	if !errors.Is(err, ErrHardConflict) {
		t.Fatalf("expected error wrapping ErrHardConflict, got: %v", err)
	}
}

func TestReview_HardConflictAccepted_WithCorrection(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor, ActorUserID: 99}

	// Same conflict, but now reviewer corrects strength to 5 mg
	action := ReviewAction{
		ExpectedRev: 1,
		CurrentDec: matchflow.RowDecision{
			Revision: 1,
		},
		Action:   "select",
		TargetID: 101,
		Corrections: []matchflow.Correction{
			{Attribute: "strength", Value: "5 mg", Reason: "supplier typo"},
		},
		Row: productmatch.Row{
			Name: "كونكور 10 مجم 30 قرص",
		},
	}

	dec, err := svc.Review(ctx, sc, action)
	if err != nil {
		t.Fatalf("expected success when hard conflict is resolved by correction, got: %v", err)
	}
	if dec.State != matchflow.StateConfirmed {
		t.Fatalf("expected StateConfirmed, got %s", dec.State)
	}
	if dec.Basis != matchflow.BasisReviewer {
		t.Fatalf("expected BasisReviewer, got %s", dec.Basis)
	}
	if dec.ProductID == nil || *dec.ProductID != 101 {
		t.Fatalf("expected product 101, got %v", dec.ProductID)
	}
	if dec.Actor == nil || dec.Actor.UserID != "99" {
		t.Fatalf("expected actor user_id 99, got %v", dec.Actor)
	}
	if len(dec.Actor.Corrections) != 1 || dec.Actor.Corrections[0].Value != "5 mg" {
		t.Fatalf("expected actor corrections preserved, got %v", dec.Actor.Corrections)
	}
}

func TestReview_Reject_Exclude_Undo(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor, ActorUserID: 99}

	prod101 := int64(101)
	currentDec := matchflow.RowDecision{
		State:     matchflow.StateAccepted,
		ProductID: &prod101,
		Revision:  1,
	}

	// 1. Reject
	rejectAction := ReviewAction{
		ExpectedRev: 1,
		CurrentDec:  currentDec,
		Action:      "reject",
		Row: productmatch.Row{
			Name: "كونكور 5 مجم 30 قرص",
		},
	}
	decReject, err := svc.Review(ctx, sc, rejectAction)
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if decReject.State != matchflow.StateNoCandidate {
		t.Errorf("expected StateNoCandidate, got %s", decReject.State)
	}
	if decReject.ProductID != nil {
		t.Errorf("expected nil ProductID after rejection, got %v", decReject.ProductID)
	}
	if decReject.Basis != matchflow.BasisReviewer {
		t.Errorf("expected BasisReviewer, got %s", decReject.Basis)
	}

	// 2. Exclude
	excludeAction := ReviewAction{
		ExpectedRev: 1,
		CurrentDec:  currentDec,
		Action:      "exclude",
		Row: productmatch.Row{
			Name: "كونكور 5 مجم 30 قرص",
		},
	}
	decExclude, err := svc.Review(ctx, sc, excludeAction)
	if err != nil {
		t.Fatalf("exclude failed: %v", err)
	}
	if decExclude.State != matchflow.StateExcluded {
		t.Errorf("expected StateExcluded, got %s", decExclude.State)
	}
	if decExclude.ProductID != nil {
		t.Errorf("expected nil ProductID after exclusion, got %v", decExclude.ProductID)
	}

	// 3. Undo
	undoAction := ReviewAction{
		ExpectedRev: decExclude.Revision,
		CurrentDec:  decExclude,
		Action:      "undo",
		Row: productmatch.Row{
			Name: "بانادول اكسترا 24 قرص",
		},
	}
	decUndo, err := svc.Review(ctx, sc, undoAction)
	if err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if decUndo.State != matchflow.StateAccepted {
		t.Errorf("expected StateAccepted after undo, got %s", decUndo.State)
	}
	if decUndo.ProductID == nil || *decUndo.ProductID != 102 {
		t.Errorf("expected ProductID 102 after undo, got %v", decUndo.ProductID)
	}
	if decUndo.Actor != nil {
		t.Errorf("expected nil Actor after undo, got %v", decUndo.Actor)
	}
}

func TestSearch_CandidatesWithDiff(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor}

	row := productmatch.Row{
		Name: "بانادول 500 مجم 24 قرص",
	}

	views, err := svc.Search(ctx, sc, "بانادول", row, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(views) == 0 {
		t.Fatal("expected at least 1 candidate, got 0")
	}

	found102 := false
	for _, v := range views {
		if v.ProductID == 102 {
			found102 = true
			if !v.Eligible {
				t.Errorf("expected candidate 102 to be eligible, got conflicts: %v", v.HardConflicts)
			}
			if v.NameAR == "" || v.Manufacturer == "" {
				t.Errorf("expected populated candidate metadata, got name=%q mfg=%q", v.NameAR, v.Manufacturer)
			}
		}
	}
	if !found102 {
		t.Errorf("expected candidate 102 in search results")
	}

	// Search directly by ID
	idViews, err := svc.Search(ctx, sc, "101", row, 5)
	if err != nil {
		t.Fatalf("Search by ID failed: %v", err)
	}
	if len(idViews) == 0 || idViews[0].ProductID != 101 {
		t.Fatalf("expected product 101 first in search by ID, got: %v", idViews)
	}
}

func TestPreviewBulk_ScopeAndExclusions(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()
	sc := Scope{EffectiveOrg: 1, Workflow: WorkflowVendor}

	row := productmatch.Row{Name: "كونكور 5 مجم 30 قرص"}
	rowFacts := productmatch.ExtractRowFacts(&row, nil)
	sig := productmatch.QuestionSignature(rowFacts, row.Name, productmatch.NormalizerVersion)

	action := ReviewAction{
		Action:   "select",
		TargetID: 101,
		Row:      row,
		RowRef: RowRef{
			QuestionSignature: sig,
		},
	}

	rows := []RowRef{
		{ID: 1, QuestionSignature: sig},
		{ID: 2, QuestionSignature: sig},
		{ID: 3, QuestionSignature: "other_signature"},
		{ID: 4, QuestionSignature: sig, ExplicitlyRejected: true}, // Excluded!
		{ID: 5, QuestionSignature: sig, State: string(matchflow.StateNoCandidate), ExplicitlyRejected: true},
	}

	preview, err := svc.PreviewBulk(ctx, sc, action, rows)
	if err != nil {
		t.Fatalf("PreviewBulk failed: %v", err)
	}

	if preview.Total != 5 {
		t.Errorf("expected Total=5, got %d", preview.Total)
	}
	if preview.Affected != 2 {
		t.Errorf("expected Affected=2 (rows 1 and 2), got %d", preview.Affected)
	}
}

func TestReview_ScopeEnforcement(t *testing.T) {
	svc, _ := newTestReviewService()
	ctx := context.Background()

	// Missing workflow
	_, err := svc.Review(ctx, Scope{}, ReviewAction{Action: "select"})
	if err == nil {
		t.Fatal("expected error for empty workflow in scope, got nil")
	}
}
