package corpus

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// TestDiagnoseMistakes classifies every wrong applied match by what separates
// the chosen product from the correct one.
//
// A diagnostic, not a gate: it prints and asserts nothing. It exists so a
// change to the scorer is aimed at a class of failure with a count beside it
// rather than at the last example somebody happened to read.
func TestDiagnoseMistakes(t *testing.T) {
	if os.Getenv("DIAGNOSE") == "" || !Available() {
		t.Skip("set DIAGNOSE=1")
	}
	for _, s := range labelledSets(t) {
		classes := map[string]int{}
		examples := map[string][]string{}
		for _, l := range s.labels {
			res := s.idx.Match(l.Row, productmatch.DefaultMatchOptions())
			if !res.Level.Settled() || res.ProductID == l.WantID {
				continue
			}
			e := productmatch.Explain(s.idx, l.Row, res.ProductID, l.WantID)
			k := classify(e)
			classes[k]++
			if len(examples[k]) < 4 {
				examples[k] = append(examples[k], fmt.Sprintf("%.2f %-42s got=%-38s want=%s",
					res.Score, trunc(l.Row.DisplayName(), 42),
					trunc(e.GotName, 38), trunc(e.WantName, 38)))
			}
		}
		t.Logf("=== %s ===", s.name)
		for _, k := range byCount(classes) {
			t.Logf("%-28s %d", k, classes[k])
			for _, e := range examples[k] {
				t.Logf("      %s", e)
			}
		}
	}
}

// TestFalseConflicts counts, per conflict kind, how often the engine finds that
// conflict between a row and the product the row IS.
//
// A conflict raised against the correct answer is a false conflict by
// definition, and it costs twice: the correct product is ranked behind whatever
// the row does not contradict, and its score is docked. The rate here is what
// says whether a discrimination rule is separating products or inventing
// differences, and it is the number to look at first when a change to the rules
// moves recall in the wrong direction.
func TestFalseConflicts(t *testing.T) {
	if os.Getenv("DIAGNOSE") == "" || !Available() {
		t.Skip("set DIAGNOSE=1")
	}
	for _, s := range labelledSets(t) {
		counts := map[string]int{}
		samples := map[string][]string{}
		for _, l := range s.labels {
			for _, kind := range productmatch.ConflictsWith(s.idx, l.Row, l.WantID) {
				counts[kind]++
				if len(samples[kind]) < 5 {
					samples[kind] = append(samples[kind],
						trunc(l.Row.DisplayName(), 46)+"  ||  "+trunc(s.idx.Name(l.WantID), 46))
				}
			}
		}
		t.Logf("=== %s (%d labels) — conflicts raised against the CORRECT product ===",
			s.name, len(s.labels))
		for _, k := range byCount(counts) {
			t.Logf("%-12s %5d  (%.1f%%)", k, counts[k],
				float64(counts[k])*100/float64(len(s.labels)))
			for _, e := range samples[k] {
				t.Logf("      %s", e)
			}
		}
	}
}

// labelledSet is one benchmark: an index and the labels scored against it.
type labelledSet struct {
	name   string
	idx    *productmatch.Index
	labels []Labelled
}

// labelledSets builds both benchmarks, which every diagnostic here needs.
func labelledSets(t *testing.T) []labelledSet {
	t.Helper()
	products, err := LoadSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	cross, err := CrossScriptLabels(products)
	if err != nil {
		t.Fatalf("cross-script labels: %v", err)
	}
	return []labelledSet{
		{"cross-script", productmatch.NewIndex(BlankEnglish(products)), cross},
		{"siblings",
			productmatch.NewIndex(append([]productmatch.MasterProduct(nil), products...)),
			SiblingLabels(products)},
	}
}

// classify names what distinguishes the correct product from the chosen one.
//
// The order is the order the fixes matter in: a difference the row itself
// states is a difference the engine had the evidence to see.
func classify(e productmatch.Explanation) string {
	switch {
	case e.StrengthDiffers:
		return "strength/combination"
	case e.NumbersDiffer:
		return "size or pack figure"
	case e.ModifierDiffers:
		return "line-extension word"
	case e.FormDiffers:
		return "dosage form"
	case e.ExtraWord != "":
		return "unmatched word: " + e.ExtraWord
	}
	return "other"
}

// byCount orders a tally, largest first, so the biggest class is read first.
func byCount(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
