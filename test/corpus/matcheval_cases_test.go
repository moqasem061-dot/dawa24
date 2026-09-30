package corpus

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
)

var updateTuningCases = flag.Bool("update", false, "update cases_tuning.jsonl.gz")

// ConvertLabelledToCases transforms cross-script and sibling labels into matcheval.Case records.
func ConvertLabelledToCases(crossLabels []Labelled, sibLabels []Labelled) []matcheval.Case {
	out := make([]matcheval.Case, 0, len(crossLabels)+len(sibLabels))

	for i, l := range crossLabels {
		diffs := []string{"cross_script"}
		if l.Family > 1 {
			diffs = append(diffs, "sibling")
		}
		c := matcheval.Case{
			ID:                  fmt.Sprintf("cs-%05d", i+1),
			Split:               "tuning",
			TemplateFamily:      "admin-cross",
			ExpressionFamily:    fmt.Sprintf("target-%d", l.WantID),
			Workflow:            []string{"vendor"},
			Language:            "en",
			RequiredGranularity: "product",
			Row: matcheval.CaseRow{
				Cells: map[string]string{
					"name": l.Row.Name,
				},
				Mapping: map[string]int{
					"name": 0,
				},
			},
			Label: matcheval.Label{
				Resolvable:  true,
				Targets:     []int64{l.WantID},
				Granularity: "product",
				Expect:      "accept",
				Difficulty:  diffs,
				Labellers:   2,
				Disputed:    false,
			},
			CatalogSnapshot: "tuning",
		}
		out = append(out, c)
	}

	for i, l := range sibLabels {
		c := matcheval.Case{
			ID:                  fmt.Sprintf("sib-%05d", i+1),
			Split:               "tuning",
			TemplateFamily:      "catalog-perturb",
			ExpressionFamily:    fmt.Sprintf("target-%d", l.WantID),
			Workflow:            []string{"vendor"},
			Language:            "ar",
			RequiredGranularity: "product",
			Row: matcheval.CaseRow{
				Cells: map[string]string{
					"name": l.Row.Name,
				},
				Mapping: map[string]int{
					"name": 0,
				},
			},
			Label: matcheval.Label{
				Resolvable:  true,
				Targets:     []int64{l.WantID},
				Granularity: "product",
				Expect:      "accept",
				Difficulty:  []string{"sibling", "close_strength"},
				Labellers:   2,
				Disputed:    false,
			},
			CatalogSnapshot: "tuning",
		}
		out = append(out, c)
	}

	return out
}

func TestGenerateTuningCases(t *testing.T) {
	outPath := filepath.Join(Dir, "cases_tuning.jsonl.gz")
	_, err := os.Stat(outPath)
	exists := err == nil

	if !*updateTuningCases && exists {
		t.Logf("cases_tuning.jsonl.gz already exists; pass -update to regenerate")
		return
	}

	products, err := LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	crossLabels, err := CrossScriptLabels(products)
	if err != nil {
		t.Fatalf("CrossScriptLabels: %v", err)
	}

	sibLabels := SiblingLabels(products)

	cases := ConvertLabelledToCases(crossLabels, sibLabels)
	if len(cases) == 0 {
		t.Fatalf("no cases converted")
	}

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create cases_tuning.jsonl.gz: %v", err)
	}
	defer func() { _ = f.Close() }()

	zw := gzip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	enc := json.NewEncoder(zw)
	for _, c := range cases {
		if err := enc.Encode(c); err != nil {
			t.Fatalf("encode case %s: %v", c.ID, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	t.Logf("successfully wrote %d tuning cases to %s", len(cases), outPath)
}
