package catalog_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// TestInferCategoriesDeterministicMatches verifies that products whose scientific names
// match known platform categories are resolved deterministically in memory without AI.
func TestInferCategoriesDeterministicMatches(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	fixture := "اسم الصنف,كود الصنف,المادة الفعالة,سعر البيع\n" +
		"أماريل 2 مجم,DET-1,أدوية السكر,35.00\n"

	session, _, err := svc.AnalyzeImport(ctx, []byte(fixture), "det.csv", 0)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeAddNewOnly,
		Options: catalog.ImportOptions{
			UseAI:          false,
			AssignCategory: true,
		},
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if prepared.Status != catalog.SessionReady {
		t.Fatalf("status = %s, want ready", prepared.Status)
	}
}

// TestInferCategoriesFrequencyBoundedAndProgress verifies that when AI is enabled,
// signals are mapped in bounded batches and do not hang even with many distinct molecules.
func TestInferCategoriesFrequencyBoundedAndProgress(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)

	mapper := &stubMapper{
		available: true,
		values: func(req catalog.ValueMapRequest) catalog.ValueMapResult {
			var matches []catalog.ValueMatch
			for _, src := range req.Sources {
				if len(req.Targets) > 0 {
					matches = append(matches, catalog.ValueMatch{
						Source:     src,
						Target:     req.Targets[0],
						Confidence: 0.95,
					})
				}
			}
			return catalog.ValueMapResult{Matches: matches}
		},
	}
	svc.SetAIMapper(mapper)
	ctx := context.Background()

	var sb strings.Builder
	sb.WriteString("اسم الصنف,كود الصنف,المادة الفعالة,سعر البيع\n")
	for i := 1; i <= 200; i++ {
		sb.WriteString(fmt.Sprintf("صنف تجريبي %d,SKU-%d,مادة فعالة فريدة %d,50.00\n", i, i, i))
	}

	session, _, err := svc.AnalyzeImport(ctx, []byte(sb.String()), "large.csv", 0)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeAddNewOnly,
		Options: catalog.ImportOptions{
			UseAI:          true,
			AssignCategory: true,
		},
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if prepared.Status != catalog.SessionReady {
		t.Fatalf("status = %s, want ready", prepared.Status)
	}

	if mapper.valueCalls > 3 {
		t.Errorf("mapper was called %d times, expected at most 3 batches", mapper.valueCalls)
	}
}
