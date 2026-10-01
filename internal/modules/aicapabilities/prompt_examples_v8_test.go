package aicapabilities

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptExamplesV8_CategoriesCoverage(t *testing.T) {
	require.Len(t, PromptExamplesV8, 10, "prompt v8 must contain exactly 10 normative synthetic examples")

	expectedCategories := map[PromptExampleCategory]bool{
		CategoryArabicShorthand:     false,
		CategoryCrossScriptNames:    false,
		CategoryStrengthDistinction: false,
		CategoryFormDistinction:     false,
		CategoryPackDistinction:     false,
		CategoryLineExtension:       false,
		CategoryManufacturerTrap:    false,
		CategoryPrefixTrap:          false,
		CategoryUnstatedAmbiguous:   false,
		CategoryAbsentNoCandidate:   false,
	}

	for _, ex := range PromptExamplesV8 {
		if _, ok := expectedCategories[ex.Category]; ok {
			expectedCategories[ex.Category] = true
		} else {
			t.Errorf("unexpected category: %s", ex.Category)
		}
	}

	for cat, found := range expectedCategories {
		assert.Truef(t, found, "category %s must be covered by prompt v8 examples", cat)
	}
}

func TestPromptExamplesV8_NoLeakageInAdversarialOrHeldout(t *testing.T) {
	// Find project root
	root := filepath.Join("..", "..", "..")
	advCasesPath := filepath.Join(root, "testdata", "matching", "adversarial", "cases.jsonl")
	f, err := os.Open(advCasesPath)
	require.NoError(t, err, "must be able to open adversarial cases.jsonl")
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var advTexts []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record struct {
			Row struct {
				Cells map[string]string `json:"cells"`
			} `json:"row"`
		}
		if err := json.Unmarshal([]byte(line), &record); err == nil {
			for _, v := range record.Row.Cells {
				if v != "" {
					advTexts = append(advTexts, strings.ToLower(v))
				}
			}
		}
	}
	require.NoError(t, scanner.Err())
	require.NotEmpty(t, advTexts)

	// Collect distinctive words/names from examples
	for _, ex := range PromptExamplesV8 {
		rawLower := strings.ToLower(ex.Item.Raw)
		// Check that the exact raw text never appears in adversarial
		for _, adv := range advTexts {
			assert.NotEqual(t, adv, rawLower, "raw text %q found in adversarial set!", ex.Item.Raw)
			assert.False(t, strings.Contains(adv, rawLower), "raw text %q contained in adversarial case %q", ex.Item.Raw, adv)
		}

		// Check catalog entry names
		for _, cat := range ex.Catalog {
			arLower := strings.ToLower(cat.AR)
			enLower := strings.ToLower(cat.EN)
			for _, adv := range advTexts {
				if arLower != "" && len([]rune(arLower)) > 4 {
					assert.False(t, strings.Contains(adv, arLower), "catalog AR %q found in adversarial case %q", cat.AR, adv)
				}
				if enLower != "" && len(enLower) > 4 {
					assert.False(t, strings.Contains(adv, enLower), "catalog EN %q found in adversarial case %q", cat.EN, adv)
				}
			}
		}
	}
}

func TestPromptExamplesV8_AnswersValidateUnderContractV8(t *testing.T) {
	for i, ex := range PromptExamplesV8 {
		t.Run(string(ex.Category), func(t *testing.T) {
			rendered := RenderExamplesV8([]PromptExampleV8{ex})
			assert.Contains(t, rendered, fmt.Sprintf("Example 1: %s", ex.Description))

			ansObj := map[string]any{
				"r": ex.Item.Ref,
				"s": ex.SelectedID,
				"e": ex.Evidence,
			}
			if ex.Abstain != "" {
				ansObj["x"] = ex.Abstain
			}
			if ex.Confidence != "" {
				ansObj["c"] = ex.Confidence
			}
			payload, err := json.Marshal(map[string]any{"a": []any{ansObj}})
			require.NoError(t, err)

			reqView := matchflow.RequestView{
				ItemOptions: map[int][]int64{ex.Item.Ref: ex.Item.Options},
			}

			valid, rejected, err := matchflow.ValidateV8(payload, reqView)
			require.NoError(t, err, "example %d must be parseable", i+1)
			assert.Empty(t, rejected, "example %d should have no validation rejections", i+1)
			require.Len(t, valid, 1, "example %d must produce 1 valid answer", i+1)
			assert.Equal(t, ex.Item.Ref, valid[0].Ref)
			if ex.SelectedID != nil {
				require.NotNil(t, valid[0].ProductID)
				assert.Equal(t, *ex.SelectedID, *valid[0].ProductID)
			} else {
				assert.Nil(t, valid[0].ProductID)
				assert.Equal(t, ex.Abstain, valid[0].Abstain)
			}
		})
	}
}
