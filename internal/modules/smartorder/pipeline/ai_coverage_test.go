package pipeline

import (
	"os"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Every row reaches the model, not only the ones the scorer found promising:
// an unmatched or needs-review line with a weak shortlist is still asked, as
// long as there is any candidate to choose from.
func TestRetrieveAsksWeakUnresolvedLines(t *testing.T) {
	m := &CandidateMatcher{index: testIndex()}
	weak := reviewFor(1, "ابلفاي")
	weak.Candidates = []productmatch.MatchCandidate{{ProductID: 101, Score: 0.10}}

	got := m.Retrieve([]Review{weak})
	if len(got) != 1 {
		t.Fatalf("a weak unresolved line was not sent to the AI (got %d)", len(got))
	}
}

// The مطابق lines the engine settled on a NAME are checked; identifier
// matches are not, because a model cannot improve on a barcode.
func TestVerifyOffersNameSettledLinesOnly(t *testing.T) {
	m := &CandidateMatcher{index: testIndex()}
	line := func(id int64, method smartorder.MatchMethod) *smartorder.Line {
		l := &smartorder.Line{ID: id, RowNumber: int(id), RawName: "ابيليفاي 10مجم"}
		Normalize([]*smartorder.Line{l})
		pid := int64(101)
		l.MatchedProductID, l.MatchMethod, l.MatchConfidence = &pid, method, 0.99
		return l
	}
	got := m.Verify([]*smartorder.Line{
		line(1, smartorder.MethodExactName),
		line(2, smartorder.MethodFuzzy),
		line(3, smartorder.MethodBarcode),
		line(4, smartorder.MethodSKU),
	})
	if len(got) != 2 {
		t.Fatalf("verified %d lines, want the 2 name-matched ones", len(got))
	}
	for _, r := range got {
		if !r.Settled || r.Line.MatchMethod == smartorder.MethodBarcode || r.Line.MatchMethod == smartorder.MethodSKU {
			t.Errorf("unexpected review: settled=%v method=%s", r.Settled, r.Line.MatchMethod)
		}
	}
}

// The run hands both populations to the AI stage. Verify existed and was
// never called, so a smart order's مطابق lines were never checked.
func TestRunSendsSettledLinesToTheAIStage(t *testing.T) {
	body, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "askable = append(askable, matcher.Verify(lines)...)") {
		t.Error("the AI stage no longer receives the settled (مطابق) lines")
	}
}
