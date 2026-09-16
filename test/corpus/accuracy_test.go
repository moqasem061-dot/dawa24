package corpus

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// accuracyBaseline records the last committed labelled result.
const accuracyBaseline = "accuracy.json"

// TestMatchAccuracy scores the engine against labels and fails on the movement
// that is always a regression: more wrong matches applied.
//
// A settled rate that goes down is reported and does not fail — refusing to
// guess is a legitimate outcome and the corpus report already tracks it. A
// precision that goes down fails, because there is no reading of that number
// under which it is an improvement.
func TestMatchAccuracy(t *testing.T) {
	if !Available() {
		t.Skip("corpus not exported; run `go run ./cmd/cli corpus-export`")
	}
	products, err := LoadSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	opts := productmatch.DefaultMatchOptions()

	// Cross-script: a Latin supplier file against an Arabic catalogue.
	crossLabels, err := CrossScriptLabels(products)
	if err != nil {
		t.Fatalf("cross-script labels: %v", err)
	}
	crossIdx := productmatch.NewIndex(BlankEnglish(products))

	// Siblings: an Arabic supplier line against the whole catalogue, restricted
	// to the brand families where a sibling can be chosen by mistake.
	sibLabels := SiblingLabels(products)
	sibIdx := productmatch.NewIndex(append([]productmatch.MasterProduct(nil), products...))

	reports := []Accuracy{
		Score("cross-script", crossIdx, crossLabels, opts),
		Score("siblings", sibIdx, sibLabels, opts),
	}

	for _, r := range reports {
		t.Log(r.Format())
		t.Logf("\n%s", r.Calibration())
		if len(r.Samples) > 0 {
			t.Logf("wrong applied matches (sample):\n%s", r.FormatSamples())
		}
	}

	if os.Getenv("CORPUS_UPDATE") != "" {
		raw, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if err := os.WriteFile(accuracyBaseline, append(raw, '\n'), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Logf("accuracy baseline updated: %s", accuracyBaseline)
		return
	}
	compareAccuracy(t, reports)
}

func compareAccuracy(t *testing.T, now []Accuracy) {
	t.Helper()
	raw, err := os.ReadFile(accuracyBaseline)
	if err != nil {
		t.Skipf("no accuracy baseline yet; run with CORPUS_UPDATE=1 to record one")
		return
	}
	var before []Accuracy
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	was := make(map[string]Accuracy, len(before))
	for _, a := range before {
		was[a.Name] = a
	}

	for _, a := range now {
		old, known := was[a.Name]
		if !known {
			t.Logf("NEW %s", a.Name)
			continue
		}
		switch {
		case a.WrongApplied > old.WrongApplied:
			t.Errorf("REGRESSION %s: wrong applied matches %d -> %d (precision %.2f%% -> %.2f%%)",
				a.Name, old.WrongApplied, a.WrongApplied, old.PrecisionPct(), a.PrecisionPct())
		case a.RightApplied < old.RightApplied:
			t.Logf("NOTE %s: correct applied matches %d -> %d", a.Name, old.RightApplied, a.RightApplied)
		default:
			t.Logf("OK %s: wrong %d -> %d, right %d -> %d",
				a.Name, old.WrongApplied, a.WrongApplied, old.RightApplied, a.RightApplied)
		}
	}
}
