package matchreview

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestMatchReview_CandidatesComponent(t *testing.T) {
	ctx := context.Background()
	row := productmatch.Row{
		Name:          "بانادول 500 مجم 24 قرص",
		Concentration: "500 mg",
		DosageForm:    "قرص",
		Unit:          "24",
		Manufacturer:  "GSK",
	}
	candidates := []matchflow.CandidateView{
		{
			ProductID:     102,
			NameAR:        "بانادول اكسترا 24 قرص",
			DosageForm:    "قرص",
			Concentration: "500 mg",
			Unit:          "24",
			Manufacturer:  "GSK",
			PublicPrice:   "35.00",
			Score:         0.92,
			Eligible:      true,
			Diff: map[string]string{
				"name":     "agree",
				"strength": "agree",
				"form":     "agree",
				"pack":     "agree",
				"brand":    "agree",
			},
		},
	}

	props := CandidatesProps{
		RowID:         1,
		Row:           row,
		Candidates:    candidates,
		PostSelectURL: "/review/select",
		ExpectedRev:   1,
	}

	var buf bytes.Buffer
	if err := Candidates(props).Render(ctx, &buf); err != nil {
		t.Fatalf("Candidates render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "dir=\"rtl\"") {
		t.Errorf("expected dir=\"rtl\" in candidates component")
	}
	if !strings.Contains(html, "<bdi>") {
		t.Errorf("expected <bdi> tags in candidates component")
	}
	if !strings.Contains(html, "aria-live=\"polite\"") {
		t.Errorf("expected aria-live=\"polite\" in candidates component")
	}
	if !strings.Contains(html, "بانادول") {
		t.Errorf("expected candidate name rendered")
	}
}

func TestMatchReview_ReasonsComponent(t *testing.T) {
	ctx := context.Background()
	props := ReasonsProps{
		Reasons: []matchflow.Reason{
			{Code: "name_exact"},
			{Code: "strength_conflict", Params: map[string]string{"attr": "strength"}},
		},
		Flags: []string{"retrieval_incomplete"},
	}

	var buf bytes.Buffer
	if err := Reasons(props).Render(ctx, &buf); err != nil {
		t.Fatalf("Reasons render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "تطابق تام في الاسم") {
		t.Errorf("expected Arabic text for name_exact")
	}
	if !strings.Contains(html, "الخاصية") || !strings.Contains(html, "التركيز") {
		t.Errorf("expected localized reason parameters")
	}
}

func TestMatchReview_FiltersComponent(t *testing.T) {
	ctx := context.Background()
	props := FiltersProps{
		CurrentState: "needs_review",
		Counts: map[string]int{
			"all":          10,
			"needs_review": 4,
		},
		FilterURL:      "/review/filter",
		TargetSelector: "#review-table",
	}

	var buf bytes.Buffer
	if err := Filters(props).Render(ctx, &buf); err != nil {
		t.Fatalf("Filters render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "aria-live=\"polite\"") {
		t.Errorf("expected aria-live=\"polite\" on counts")
	}
	if !strings.Contains(html, "<bdi>4</bdi>") {
		t.Errorf("expected count 4 in <bdi>")
	}
}

func TestMatchReview_BulkPreviewModal(t *testing.T) {
	ctx := context.Background()
	props := BulkPreviewProps{
		Total:       50,
		Affected:    12,
		TargetName:  "كونكور 5 مجم 30 قرص",
		TargetID:    101,
		Action:      "select",
		ConfirmURL:  "/review/bulk",
		ExpectedRev: 1,
	}

	var buf bytes.Buffer
	if err := BulkPreviewModal(props).Render(ctx, &buf); err != nil {
		t.Fatalf("BulkPreviewModal render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<bdi>12</bdi>") {
		t.Errorf("expected affected count 12 in <bdi>")
	}
	if !strings.Contains(html, "FR-046") {
		t.Errorf("expected FR-046 safety guarantee note")
	}
}

func TestMatchReview_CorrectionFormComponent(t *testing.T) {
	ctx := context.Background()
	props := CorrectionFormProps{
		RowID: 10,
		Row: productmatch.Row{
			Name:          "دواء تجريبي",
			Concentration: "100 mg",
		},
		PostURL:     "/review/correct",
		ExpectedRev: 2,
	}

	var buf bytes.Buffer
	if err := CorrectionForm(props).Render(ctx, &buf); err != nil {
		t.Fatalf("CorrectionForm render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "corr_strength") {
		t.Errorf("expected corr_strength input field")
	}
	if !strings.Contains(html, "100 mg") {
		t.Errorf("expected existing concentration in input value")
	}
}

func TestMatchReview_SearchComponent(t *testing.T) {
	ctx := context.Background()
	props := SearchProps{
		RowID:       5,
		SearchURL:   "/catalog/search",
		Placeholder: "ابحث بالاسم...",
	}

	var buf bytes.Buffer
	if err := Search(props).Render(ctx, &buf); err != nil {
		t.Fatalf("Search render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "search_target_5") {
		t.Errorf("expected combobox input with name search_target_5")
	}
}

func TestMatchReview_KeyboardComponent(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	if err := Keyboard(KeyboardProps{Class: "my-custom-class"}).Render(ctx, &buf); err != nil {
		t.Fatalf("Keyboard render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "dawaMatchReviewKeyboard()") {
		t.Errorf("expected x-data=dawaMatchReviewKeyboard() in keyboard component")
	}
	if !strings.Contains(html, "اختصارات لوحة المفاتيح") {
		t.Errorf("expected Arabic keyboard shortcuts title")
	}
	if !strings.Contains(html, "<kbd") {
		t.Errorf("expected <kbd> elements in keyboard shortcuts help")
	}
}

func TestMatchReview_AIStatusComponent(t *testing.T) {
	ctx := context.Background()

	// 1. Enhancing state
	enhancingProps := AIStatusProps{
		State:       "enhancing",
		RunID:       42,
		Current:     10,
		Total:       50,
		StopURL:     "/catalog/matching/stop",
		ContinueURL: "/catalog/matching/review",
		CSRFToken:   "csrf-secret-123",
	}

	var buf bytes.Buffer
	if err := AIStatus(enhancingProps).Render(ctx, &buf); err != nil {
		t.Fatalf("AIStatus enhancing render failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "جاري التحسين الذكي") {
		t.Errorf("expected Arabic enhancing title")
	}
	if !strings.Contains(html, "10/50") {
		t.Errorf("expected 10/50 progress count")
	}
	if !strings.Contains(html, "/catalog/matching/stop") {
		t.Errorf("expected stop button form action")
	}
	if !strings.Contains(html, "متابعة بدون انتظار") {
		t.Errorf("expected continue without waiting link")
	}

	// 2. Completed state with zero improvement
	completedProps := AIStatusProps{
		State:       "completed",
		RunID:       42,
		AcceptedNew: 0,
		Confirmed:   5,
		Abstained:   2,
		Rejected:    1,
		Failed:      0,
		Unattempted: 3,
	}

	buf.Reset()
	if err := AIStatus(completedProps).Render(ctx, &buf); err != nil {
		t.Fatalf("AIStatus completed render failed: %v", err)
	}

	html = buf.String()
	if !strings.Contains(html, "لا يوجد تحسين (No improvement)") {
		t.Errorf("expected No improvement badge when AcceptedNew == 0")
	}
	if !strings.Contains(html, "مطابقات مقبولة جديدة") {
		t.Errorf("expected new accepted counter label")
	}
	if !strings.Contains(html, "New Accepted") {
		t.Errorf("expected English subtitle for new accepted")
	}
}
